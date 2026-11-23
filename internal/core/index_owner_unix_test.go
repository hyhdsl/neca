//go:build !windows

package core

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWriteAtomicPreservesIndexOwnership(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.txt")
	if err := os.WriteFile(path, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(path, 0, 10001); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewFileIndexStore(path, filepath.Join(dir, ".lock"))
	if err := store.writeAtomic(nil); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatalf("mode changed: %v -> %v", before.Mode().Perm(), after.Mode().Perm())
	}
	bst, ok := before.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("stat_t not available")
	}
	ast, ok := after.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("stat_t not available")
	}
	if bst.Uid != ast.Uid || bst.Gid != ast.Gid {
		t.Fatalf("owner changed: %d:%d -> %d:%d", bst.Uid, bst.Gid, ast.Uid, ast.Gid)
	}
}
