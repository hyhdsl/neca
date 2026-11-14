package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// TestParseIndexData 校验 6 字段解析、空行跳过与顺序保留。
func TestParseIndexData(t *testing.T) {
	data := []byte("V\t271114200648Z\t\tABC123\tunknown\tC = CN, O = Org, CN = TMP01\n" +
		"\n" +
		"R\t271114200648Z\t261114200648Z\tDEF456\tunknown\t/CN=TMP02\n")
	rows, err := parseIndexData(data)
	if err != nil {
		t.Fatalf("parseIndexData() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("行数 = %d, want 2", len(rows))
	}
	if rows[0].Serial != "ABC123" || rows[0].Status != ports.IndexValid {
		t.Fatalf("第一行 = %+v", rows[0])
	}
	if rows[1].Serial != "DEF456" || rows[1].Status != ports.IndexRevoked || rows[1].RevokedAt != "261114200648Z" {
		t.Fatalf("第二行 = %+v", rows[1])
	}
}

// TestParseIndexDataRejectsBadRow 校验坏行带行号报错。
func TestParseIndexDataRejectsBadRow(t *testing.T) {
	if _, err := parseIndexData([]byte("V\tbroken\n")); err == nil {
		t.Fatal("坏行应当报错")
	}
}

// TestFileIndexStoreUpdate 校验追加、回滚与原子落盘。
func TestFileIndexStoreUpdate(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.txt")
	if err := os.WriteFile(indexPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewFileIndexStore(indexPath, filepath.Join(dir, ".lock"))
	ctx := context.Background()

	if err := store.Update(ctx, func(idx *ports.Index) error {
		idx.Rows = append(idx.Rows, ports.Row{
			Status: ports.IndexValid, Expires: "271114200648Z", Serial: "ABC123",
			File: "unknown", Subject: "C = CN, O = Org, CN = TMP01",
		})
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	rows, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rows) != 1 || rows[0].Serial != "ABC123" {
		t.Fatalf("Load() = %+v", rows)
	}

	// 回滚：fn 返回错误时文件保持原样。
	boom := errors.New("boom")
	if err := store.Update(ctx, func(idx *ports.Index) error {
		idx.Rows = nil
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("Update() error = %v, want boom", err)
	}
	rows, _ = store.Load(ctx)
	if len(rows) != 1 {
		t.Fatalf("回滚后行数 = %d, want 1", len(rows))
	}
}

// TestFileIndexStoreLockConflict 校验两个实例竞争同一把锁时返回 ErrIndexLocked。
func TestFileIndexStoreLockConflict(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.txt")
	if err := os.WriteFile(indexPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	store1 := NewFileIndexStore(indexPath, filepath.Join(dir, ".lock"))
	store2 := NewFileIndexStore(indexPath, filepath.Join(dir, ".lock"))
	ctx := context.Background()

	err := store1.Update(ctx, func(idx *ports.Index) error {
		err := store2.Update(ctx, func(*ports.Index) error { return nil })
		if !errors.Is(err, ports.ErrIndexLocked) {
			return fmt.Errorf("第二个实例应返回锁定错误，实际: %v", err)
		}
		idx.Rows = append(idx.Rows, ports.Row{Status: ports.IndexValid, Serial: "ABC123"})
		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	rows, _ := store2.Load(ctx)
	if len(rows) != 1 {
		t.Fatalf("锁释放后行数 = %d, want 1", len(rows))
	}
}
