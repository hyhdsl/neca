package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseMeta 校验键顺序、未知键保留与坏行跳过。
func TestParseMeta(t *testing.T) {
	input := []byte("CN=TMP01\nPLATFORMS=windows,android\n\n# 注释\nBROKEN\nSERIAL=ABC123\n")
	meta, err := ParseMeta(input)
	if err != nil {
		t.Fatalf("ParseMeta() error = %v", err)
	}
	if got := meta.Get("CN"); got != "TMP01" {
		t.Fatalf("CN = %q", got)
	}
	if got := meta.Get("PLATFORMS"); got != "windows,android" {
		t.Fatalf("PLATFORMS = %q", got)
	}
	if got := meta.Get("SERIAL"); got != "ABC123" {
		t.Fatalf("SERIAL = %q", got)
	}
	want := []string{"CN", "PLATFORMS", "SERIAL"}
	keys := meta.Keys()
	if len(keys) != len(want) {
		t.Fatalf("Keys() = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("Keys() = %v, want %v", keys, want)
		}
	}
}

// TestWriteLegacyMeta 校验旧脚本字段与顺序逐字节一致。
func TestWriteLegacyMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TMP01.env")
	err := WriteLegacyMeta(path, "TMP01", "windows,android", "2026-11-15", "Nov 14 20:06:48 2027 GMT", "ABC123")
	if err != nil {
		t.Fatalf("WriteLegacyMeta() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 meta 失败：%v", err)
	}
	want := "CN=TMP01\nPLATFORMS=windows,android\nCREATED=2026-11-15\nEXPIRES=Nov 14 20:06:48 2027 GMT\nSERIAL=ABC123\n"
	if string(data) != want {
		t.Fatalf("meta 内容 = %q, want %q", data, want)
	}
}

// TestLoadMetaMissing 校验缺失文件返回 (nil, nil)，由调用方决定默认值。
func TestLoadMetaMissing(t *testing.T) {
	meta, err := LoadMeta(filepath.Join(t.TempDir(), "nope.env"))
	if err != nil {
		t.Fatalf("LoadMeta() error = %v", err)
	}
	if meta != nil {
		t.Fatalf("LoadMeta() = %+v, want nil", meta)
	}
}
