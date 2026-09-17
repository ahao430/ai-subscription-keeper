package api

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "1.0.0", 0},
		{"v1.2.0", "v1.1.9", 1},
		{"1.0.9", "v1.1", -1},
		{"v2.0", "v1.9.9", 1},
		{"v1.0.0-rc1", "v1.0.0", 0}, // 预发布后缀忽略
		{"", "0.0.1", -1},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestExtractBinaryFromArchive(t *testing.T) {
	// 构造一个 tar.gz，内含目录 + keeper 可执行文件
	tmp := t.TempDir()
	arch := filepath.Join(tmp, "keeper_test_darwin_arm64.tar.gz")
	f, err := os.Create(arch)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	content := []byte("#!/bin/sh\nfake keeper binary\n")
	tw.WriteHeader(&tar.Header{Name: "keeper_test_darwin_arm64/keeper", Mode: 0o755, Size: int64(len(content))})
	tw.Write(content)
	// 干扰项：README
	rd := []byte("readme")
	tw.WriteHeader(&tar.Header{Name: "keeper_test_darwin_arm64/README.md", Mode: 0o644, Size: int64(len(rd))})
	tw.Write(rd)
	tw.Close()
	gz.Close()
	f.Close()

	got, err := extractBinaryFromArchive(arch, "keeper_test_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("extracted content mismatch: %q", got)
	}
}

func TestExpectedAssetName(t *testing.T) {
	name := expectedAssetName()
	want := "keeper_" + runtime.GOOS + "_"
	if len(name) <= len(want) || name[:len(want)] != want {
		t.Fatalf("asset name %q should start with %q", name, want)
	}
}

func TestParseVersionOutput(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2026/09/17 10:00:00 ai-subscription-keeper v1.2.1\n", "v1.2.1"},
		{"2026/09/17 10:00:00 ai-subscription-keeper dev\n", "dev"},
		{"", ""},
		{"\n\n", ""},
	}
	for _, c := range cases {
		if got := parseVersionOutput(c.in); got != c.want {
			t.Errorf("parseVersionOutput(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestVerifyNewBinary(t *testing.T) {
	// 用一个输出合法版本号的 shell 脚本模拟新二进制
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "fakekeeper")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho '2026/09/17 ai-subscription-keeper v9.9.9'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("unix 脚本模拟仅在非 Windows 运行")
	}
	if err := verifyNewBinary(bin, "v9.9.9"); err != nil {
		t.Fatalf("合法二进制不应报错: %v", err)
	}
	if err := verifyNewBinary(bin, "v1.0.0"); err == nil {
		t.Fatal("版本不匹配应当报错")
	}
	// 损坏文件：无执行权限/非可执行内容 → 必须报错且不得替换线上文件
	if err := os.WriteFile(bin, []byte("not an executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyNewBinary(bin, "v9.9.9"); err == nil {
		t.Fatal("损坏文件应当报错")
	}
}
