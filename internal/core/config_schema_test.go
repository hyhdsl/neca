// 本文件测试配置 schema v2 的严格校验、v1 兼容与迁移。
package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeSchemaFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "neca.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadSchemaError(t *testing.T, path string) error {
	t.Helper()
	_, err := (JSONSource{Path: path}).Load(context.Background())
	return err
}

func TestExampleConfigJSONIsValidV2(t *testing.T) {
	data, err := ExampleConfigJSON()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeStrictFileConfig(data, "example"); err != nil {
		t.Fatalf("example config is not valid v2: %v", err)
	}
}

func TestSchemaV2StrictValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "unknown top-level", body: `{"schemaVersion":2,"unknown":true}`, want: "unknown field"},
		{name: "unknown nested", body: `{"schemaVersion":2,"web":{"unknown":true}}`, want: "unknown field"},
		{name: "duplicate top-level", body: `{"schemaVersion":2,"schemaVersion":2}`, want: "重复字段"},
		{name: "duplicate nested", body: `{"schemaVersion":2,"web":{"enabled":false,"enabled":true}}`, want: "重复字段"},
		{name: "relative path", body: `{"schemaVersion":2,"paths":{"pkiDir":"pki"}}`, want: "绝对路径"},
		{name: "invalid key bits", body: `{"schemaVersion":2,"defaults":{"keyBits":1024}}`, want: "keyBits"},
		{name: "invalid ttl", body: `{"schemaVersion":2,"download":{"ttl":"bad"}}`, want: "download.ttl"},
		{name: "non-loopback web", body: `{"schemaVersion":2,"web":{"listen":"0.0.0.0:18080"}}`, want: "web.listen"},
		{name: "trailing object", body: `{"schemaVersion":2} {}`, want: "只允许一个 JSON 对象"},
		{name: "unsupported version", body: `{"schemaVersion":3}`, want: "不支持的 schemaVersion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loadSchemaError(t, writeSchemaFile(t, tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestSchemaV2Valid(t *testing.T) {
	body := `{
  "schemaVersion": 2,
  "serverId": "vpn.example.com",
  "paths": {"pkiDir": "/tmp/pki"},
  "defaults": {"days": 30, "keyBits": 2048},
  "web": {"enabled": false, "listen": "127.0.0.1:18080", "readOnly": true}
}`
	if err := loadSchemaError(t, writeSchemaFile(t, body)); err != nil {
		t.Fatalf("valid v2 config rejected: %v", err)
	}
}

func TestSchemaV1RemainsLenient(t *testing.T) {
	for _, body := range []string{
		`{"schemaVersion":1,"unknown":{"x":1},"web":{"unknown":true}}`,
		`{"serverId":"vpn.example.com","unknown":true}`,
		`{"schemaVersion":1,"paths":{"pkiDir":"pki"}}`,
	} {
		if err := loadSchemaError(t, writeSchemaFile(t, body)); err != nil {
			t.Fatalf("v1 compatibility failed: %v", err)
		}
	}
}

func TestMigrateConfigV1ToV2(t *testing.T) {
	path := writeSchemaFile(t, `{
  "schemaVersion": 1,
  "serverId": "vpn.example.com",
  "web": {"enabled": false}
}
`)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := MigrateConfigFile(path, false)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !result.NeedsMigration || result.Written || result.From != 1 || result.To != 2 {
		t.Fatalf("check result = %+v", result)
	}
	if data, _ := os.ReadFile(path); string(data) != string(original) {
		t.Fatal("check must not write")
	}

	result, err = MigrateConfigFile(path, true)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !result.Written || result.BackupPath == "" {
		t.Fatalf("write result = %+v", result)
	}
	backup, err := os.ReadFile(result.BackupPath)
	if err != nil || string(backup) != string(original) {
		t.Fatalf("backup = %q, err=%v", backup, err)
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migrated), `"schemaVersion": 2`) {
		t.Fatalf("migrated config = %s", migrated)
	}
	if _, err := decodeStrictFileConfig(migrated, path); err != nil {
		t.Fatalf("migrated config is not valid v2: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestMigrateConfigRefusesUnknownFields(t *testing.T) {
	path := writeSchemaFile(t, `{"schemaVersion":1,"unknown":true}`)
	original, _ := os.ReadFile(path)
	if _, err := MigrateConfigFile(path, true); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
	if data, _ := os.ReadFile(path); string(data) != string(original) {
		t.Fatal("failed migration modified config")
	}
	if matches, _ := filepath.Glob(path + ".schema-v1-*.bak"); len(matches) != 0 {
		t.Fatalf("unexpected backups: %v", matches)
	}
}

func TestMigrateConfigRefusesDuplicateKeys(t *testing.T) {
	path := writeSchemaFile(t, `{"schemaVersion":1,"serverId":"a","serverId":"b"}`)
	if _, err := MigrateConfigFile(path, true); err == nil || !strings.Contains(err.Error(), "重复字段") {
		t.Fatalf("error = %v, want duplicate key error", err)
	}
}

func TestMigrateConfigAlreadyV2(t *testing.T) {
	path := writeSchemaFile(t, `{"schemaVersion":2,"serverId":"vpn.example.com"}`)
	result, err := MigrateConfigFile(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.NeedsMigration || result.Written || result.BackupPath != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMigrateConfigRejectsNull(t *testing.T) {
	path := writeSchemaFile(t, "null")
	if _, err := MigrateConfigFile(path, true); err == nil || !strings.Contains(err.Error(), "JSON 对象") {
		t.Fatalf("error = %v, want JSON object error", err)
	}
}

func TestMigrateConfigRejectsMalformedAndUnsupported(t *testing.T) {
	if _, err := MigrateConfigFile(writeSchemaFile(t, "{bad"), true); err == nil {
		t.Fatal("malformed config must fail")
	}
	if _, err := MigrateConfigFile(writeSchemaFile(t, `{"schemaVersion":3}`), true); err == nil {
		t.Fatal("schema 3 must fail")
	}
}

func TestUpdateWebConfigV2RejectsUnknownFields(t *testing.T) {
	path := writeSchemaFile(t, `{"schemaVersion":2,"unknown":true,"web":{"enabled":false}}`)
	if _, err := UpdateWebReadOnly(path, false); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}
