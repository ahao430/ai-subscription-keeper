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
