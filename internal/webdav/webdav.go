// Package webdav implements the minimal WebDAV client subset needed for
// config backup sync: PUT / GET a file, MKCOL a directory, PROPFIND to test
// connectivity. Authentication is HTTP Basic.
package webdav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	Server   string // e.g. https://dav.jianguoyun.com/dav/
	Username string
	Password string
}

func (c *Client) hc() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

// url joins the server base with an absolute remote path.
func (c *Client) url(path string) string {
	return strings.TrimRight(c.Server, "/") + "/" + strings.TrimLeft(path, "/")
}

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path), rdr)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.Username, c.Password)
	return req, nil
}

// Test verifies connectivity and credentials with a Depth-0 PROPFIND on the
// server root.
func (c *Client) Test(ctx context.Context) error {
	req, err := c.newRequest(ctx, "PROPFIND", "/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Depth", "0")
	resp, err := c.hc().Do(req)
	if err != nil {
		return fmt.Errorf("连接 WebDAV 服务器失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode == 401 {
		return fmt.Errorf("认证失败（401）：请检查用户名和密码")
	}
	if resp.StatusCode/100 != 2 && resp.StatusCode != 207 {
		return fmt.Errorf("WebDAV 响应异常: HTTP %d", resp.StatusCode)
	}
	return nil
}

// EnsureDir creates the remote directory (MKCOL); existing dir (405/301) is OK.
func (c *Client) EnsureDir(ctx context.Context, path string) error {
	path = "/" + strings.Trim(path, "/") + "/"
	if path == "/" {
		return nil
	}
	req, err := c.newRequest(ctx, "MKCOL", path, nil)
	if err != nil {
		return err
	}
	resp, err := c.hc().Do(req)
	if err != nil {
		return fmt.Errorf("创建远程目录失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	// 201 created / 405 already exists / 301 redirect (some providers) are fine.
	if resp.StatusCode != 201 && resp.StatusCode != 405 && resp.StatusCode/100 == 3 {
		return nil
	}
	if resp.StatusCode/100 != 2 && resp.StatusCode != 405 && resp.StatusCode/100 != 3 {
		return fmt.Errorf("创建远程目录失败: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Put uploads data to the remote path.
func (c *Client) Put(ctx context.Context, path string, data []byte) error {
	req, err := c.newRequest(ctx, http.MethodPut, path, data)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.hc().Do(req)
	if err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("上传失败: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Get downloads the remote path; returns error on 404.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc().Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("远程备份文件不存在（404）")
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}
