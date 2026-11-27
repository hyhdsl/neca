// 本文件测试索引/CRL 替换对单文件 bind mount 的兼容逻辑。
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFileRegularPath(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target.txt")
	tmp := filepath.Join(dir, "tmp.txt")
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(tmp, dst); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "new" {
		t.Fatalf("target = %q, %v", data, err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temporary file still exists: %v", err)
	}
}

func TestCopyFileIntoExisting(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target.txt")
	tmp := filepath.Join(dir, "tmp.txt")
	if err := os.WriteFile(dst, []byte("old-content"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFileIntoExisting(tmp, dst); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "new" {
		t.Fatalf("target = %q, %v", data, err)
	}
}
