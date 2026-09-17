package api

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/version"
)

// GitHubRelease is the subset of the GitHub releases API we consume.
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

const githubRepo = "ahao430/ai-subscription-keeper"

// fetchRelease retrieves the latest release (or a specific tag) via the
// proxy-aware HTTP client. GitHub is unreachable in some networks without a
// proxy, hence a.HC rather than a bare http.Client.
func fetchRelease(ctx context.Context, a *app.App, tag string) (*GitHubRelease, error) {
	url := "https://api.github.com/repos/" + githubRepo + "/releases/latest"
	if tag != "" {
		url = "https://api.github.com/repos/" + githubRepo + "/releases/tags/" + tag
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ai-subscription-keeper")
	resp, err := a.HC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("访问 GitHub 失败（可在系统设置配置代理后重试）: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusNotFound {
		return nil, errStr("暂无已发布的 Release 版本")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}
	var rel GitHubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("解析 Release 信息失败: %w", err)
	}
	return &rel, nil
}

// compareSemver compares dotted version strings ("v1.2.3" / "1.2.3"),
// returning >0 when a is newer, 0 when equal, <0 when b is newer.
func compareSemver(a, b string) int {
	parse := func(s string) []int {
		s = strings.TrimPrefix(strings.TrimSpace(s), "v")
		// strip pre-release/metadata suffix
		if i := strings.IndexAny(s, "-+"); i >= 0 {
			s = s[:i]
		}
		parts := strings.Split(s, ".")
		out := make([]int, len(parts))
		for i, p := range parts {
			out[i], _ = strconv.Atoi(p)
		}
		return out
	}
	pa, pb := parse(a), parse(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// expectedAssetName returns the goreleaser archive name for this platform,
// e.g. keeper_darwin_arm64.tar.gz / keeper_windows_amd64.zip.
func expectedAssetName() string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("keeper_%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
}

// handleCheckUpdate compares the running version against the latest GitHub release.
func handleCheckUpdate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		rel, err := fetchRelease(ctx, a, "")
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		current := version.Version
		available := current != "dev" && compareSemver(rel.TagName, current) > 0
		// 探测磁盘上的二进制版本：若比运行中进程新，说明上次一键升级已完成
		// 文件替换但进程重启失败，提示用户手动重启而不是再次升级。
		disk := diskBinaryVersion()
		pendingRestart := disk != "" && disk != "dev" && compareSemver(disk, current) > 0
		notes := rel.Body
		if len(notes) > 600 {
			notes = notes[:600] + "…"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"current_version":   current,
			"latest_version":    rel.TagName,
			"update_available":  available,
			"release_url":       rel.HTMLURL,
			"published_at":      rel.PublishedAt,
			"notes":             notes,
			"asset_for_platform": expectedAssetName(),
			"disk_version":      disk,
			"pending_restart":   pendingRestart,
		})
	}
}

// diskBinaryVersion runs the on-disk executable with -version and returns the
// version it reports ("dev" for local builds, "" when the probe fails).
func diskBinaryVersion() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// -version 的输出走 log（stderr），必须用 CombinedOutput 捕获
	out, err := exec.CommandContext(ctx, exe, "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	return parseVersionOutput(string(out))
}

// parseVersionOutput extracts the trailing version token from the log line
// printed by `keeper -version` (e.g. "2026/09/17 ... ai-subscription-keeper v1.2.1").
func parseVersionOutput(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// verifyNewBinary executes the freshly written binary with -version and
// requires it to report the expected release version. This catches truncated
// downloads, wrong-architecture archives and broken executables BEFORE the
// running binary is touched, so a failed upgrade can never take the service down.
func verifyNewBinary(path, wantVersion string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("新版本可执行文件校验失败（下载可能不完整）: %v: %s", err, truncateText(string(out), 200))
	}
	got := parseVersionOutput(string(out))
	if got != strings.TrimPrefix(wantVersion, "v") && got != wantVersion {
		return fmt.Errorf("新版本校验失败: 期望 %s，实际输出 %q", wantVersion, got)
	}
	return nil
}

func truncateText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// extractBinaryFromArchive pulls the "keeper" executable out of a tar.gz/zip
// release archive and returns its bytes.
func extractBinaryFromArchive(archivePath, name string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.OpenReader(archivePath)
		if err != nil {
			return nil, fmt.Errorf("打开 zip 失败: %w", err)
		}
		defer zr.Close()
		for _, f := range zr.File {
			if filepath.Base(f.Name) == "keeper"+exeSuffix() && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 256<<20))
			}
		}
		return nil, errors.New("压缩包中未找到 keeper 可执行文件")
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("解压 gzip 失败: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("压缩包中未找到 keeper 可执行文件")
		}
		if err != nil {
			return nil, fmt.Errorf("读取 tar 失败: %w", err)
		}
		if filepath.Base(hdr.Name) == "keeper"+exeSuffix() && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, 256<<20))
		}
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// restartSelf replaces the current process with the newly installed binary.
func restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		// Windows 不能替换运行中的进程映像：启动新进程后退出。
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := cmd.Start(); err != nil {
			return err
		}
		os.Exit(0)
	}
	// Unix: exec 原地替换进程映像，保留 PID / nohup / supervisor 关系。
	return syscall.Exec(exe, append([]string{exe}, os.Args[1:]...), os.Environ())
}

// handleUpgrade downloads the specified (or latest) release, replaces the
// running binary and restarts the process.
func handleUpgrade(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tag string `json:"tag"`
		}
		if r.Body != nil {
			_ = decodeBody(r, &body)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		rel, err := fetchRelease(ctx, a, body.Tag)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		want := expectedAssetName()
		var assetURL string
		for _, as := range rel.Assets {
			if as.Name == want {
				assetURL = as.BrowserDownloadURL
				break
			}
		}
		if assetURL == "" {
			writeError(w, http.StatusNotFound, fmt.Errorf("Release %s 中未找到本平台产物 %s", rel.TagName, want))
			return
		}

		// 下载压缩包到临时文件
		tmpArchive, err := os.CreateTemp("", "keeper-upgrade-*.bin")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		tmpArchivePath := tmpArchive.Name()
		defer os.Remove(tmpArchivePath)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
		req.Header.Set("User-Agent", "ai-subscription-keeper")
		resp, err := a.HC.Do(req)
		if err != nil {
			tmpArchive.Close()
			writeError(w, http.StatusBadGateway, fmt.Errorf("下载失败: %w", err))
			return
		}
		if resp.StatusCode != http.StatusOK {
			tmpArchive.Close()
			writeError(w, http.StatusBadGateway, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode))
			return
		}
		_, cpErr := io.Copy(tmpArchive, io.LimitReader(resp.Body, 512<<20))
		resp.Body.Close()
		tmpArchive.Close()
		if cpErr != nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("下载不完整: %w", cpErr))
			return
		}

		// 解出可执行文件
		binBytes, err := extractBinaryFromArchive(tmpArchivePath, want)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		exePath, err := os.Executable()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		exePath, _ = filepath.EvalSymlinks(exePath)

		// 写入新文件 → 校验可执行 → 备份旧文件 → 原子替换
		newPath := exePath + ".new"
		if err := os.WriteFile(newPath, binBytes, 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("写入新版本失败（Docker 内通常只读，请拉取新镜像）: %w", err))
			return
		}
		// 动正在运行的服务之前先验证新文件能正常执行，坏包绝不落地。
		if err := verifyNewBinary(newPath, rel.TagName); err != nil {
			os.Remove(newPath)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		bakPath := exePath + ".old"
		os.Remove(bakPath)
		if err := os.Rename(exePath, bakPath); err != nil {
			os.Remove(newPath)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("备份旧版本失败: %w", err))
			return
		}
		if err := os.Rename(newPath, exePath); err != nil {
			os.Rename(bakPath, exePath) // 尽力回滚
			writeError(w, http.StatusInternalServerError, fmt.Errorf("替换可执行文件失败: %w", err))
			return
		}
		os.Chmod(exePath, 0o755)

		// 先响应客户端，再延迟重启进程。
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"old_version": version.Version,
			"new_version": rel.TagName,
			"restarting":  true,
		})
		go func() {
			time.Sleep(1 * time.Second)
			if err := restartSelf(); err != nil {
				log.Printf("[upgrade] 自动重启失败（新版本文件已就位，请在「检查更新」中按提示手动重启）: %v", err)
			}
		}()
	}
}
