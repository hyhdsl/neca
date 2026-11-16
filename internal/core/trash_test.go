package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrashListAndPurge(t *testing.T) {
	cfg := &Config{PKIDir: t.TempDir()}
	entry := TrashEntry{ID: "20260101T000000Z-TMP01", CN: "TMP01", CreatedAt: time.Now().UTC()}
	dir := filepath.Join(TrashDir(cfg), entry.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entry.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := ListTrash(cfg)
	if err != nil || len(entries) != 1 || entries[0].CN != "TMP01" {
		t.Fatalf("ListTrash = %+v, %v", entries, err)
	}
	if err := PurgeTrash(cfg, entry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("trash entry still exists: %v", err)
	}
}

func TestExpiredTrashIDs(t *testing.T) {
	cfg := &Config{PKIDir: t.TempDir(), TrashRetention: time.Hour}
	now := time.Now().UTC()
	for id, created := range map[string]time.Time{
		"old": now.Add(-2 * time.Hour),
		"new": now.Add(-30 * time.Minute),
	} {
		dir := filepath.Join(TrashDir(cfg), id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(TrashEntry{ID: id, CN: id, CreatedAt: created})
		if err := os.WriteFile(filepath.Join(dir, "entry.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := ExpiredTrashIDs(cfg, now)
	if err != nil || len(ids) != 1 || ids[0] != "old" {
		t.Fatalf("expired = %v, %v; want [old]", ids, err)
	}
	cfg.TrashRetention = 0
	if ids, err := ExpiredTrashIDs(cfg, now); err != nil || len(ids) != 0 {
		t.Fatalf("unlimited retention = %v, %v", ids, err)
	}
}

func TestMoveTrashFilesRoundTrip(t *testing.T) {
	cfg := &Config{PKIDir: t.TempDir()}
	source := filepath.Join(cfg.PKIDir, "TMP01.crt")
	if err := os.WriteFile(source, []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := TrashEntry{ID: "20260101T000000Z-TMP01", Files: []TrashFile{{Source: source, Relative: filepath.Join("files", "0000")}}}
	if err := moveTrashFiles(cfg, entry, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists after move: %v", err)
	}
	if err := moveTrashFiles(cfg, entry, false); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "cert" {
		t.Fatalf("restored file = %q, %v", data, err)
	}
}
