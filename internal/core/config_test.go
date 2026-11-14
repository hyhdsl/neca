package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mapSource 是测试用的配置层。
type mapSource map[string]string

func (mapSource) Name() string { return "test" }

func (s mapSource) Load(context.Context) (map[string]string, error) { return s, nil }

// TestEnvSourceIgnoresEmpty 校验空环境变量按未设置处理（bash :- 语义）。
func TestEnvSourceIgnoresEmpty(t *testing.T) {
	source := EnvSource{Lookup: func(key string) (string, bool) {
		switch key {
		case "PKI_DIR":
			return "", true // 空值必须被忽略
		case "ORG":
			return "TestOrg", true
		default:
			return "", false
		}
	}}
	values, err := source.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, exists := values["PKI_DIR"]; exists {
		t.Fatalf("空值不应进入配置层：%v", values)
	}
	if values["ORG"] != "TestOrg" {
		t.Fatalf("ORG = %q", values["ORG"])
	}
}

// TestFileSource 校验 KEY=VALUE 解析与注释/空行跳过。
func TestFileSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "neca.conf")
	content := "# 注释\nPKI_DIR=/tmp/pki\n\nORG = Test Org \n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	values, err := FileSource{Path: path}.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if values["PKI_DIR"] != "/tmp/pki" || values["ORG"] != "Test Org" {
		t.Fatalf("解析结果 = %v", values)
	}
}

// TestLoadConfigLayout 校验默认值推导、目录骨架与 CA 校验。
func TestLoadConfigLayout(t *testing.T) {
	root := t.TempDir()
	pki := filepath.Join(root, "pki")
	if err := os.MkdirAll(pki, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pki, "ca.crt"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFrom(mapSource{"PKI_DIR": pki, "DEFAULT_DAYS": "30"})
	if err != nil {
		t.Fatalf("LoadConfigFrom() error = %v", err)
	}
	if cfg.CACert != filepath.Join(pki, "ca.crt") {
		t.Fatalf("CACert = %q", cfg.CACert)
	}
	if cfg.IndexFile != filepath.Join(pki, "ca", "index.txt") {
		t.Fatalf("IndexFile = %q", cfg.IndexFile)
	}
	if cfg.DefaultDays != 30 {
		t.Fatalf("DefaultDays = %d", cfg.DefaultDays)
	}
	if cfg.ORG != "ExampleOrg" {
		t.Fatalf("ORG = %q", cfg.ORG)
	}

	// 目录骨架与初始文件。
	for _, dir := range []string{cfg.CADBDir, cfg.ClientsDir, cfg.MetaDir, cfg.ArchiveDir, cfg.ExportDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("目录未创建: %s (%v)", dir, err)
		}
	}
	if data, err := os.ReadFile(cfg.CRLNumFile); err != nil || string(data) != "1000\n" {
		t.Fatalf("crlnumber = %q, %v", data, err)
	}
	conf, err := os.ReadFile(cfg.CAConf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conf), "database = "+cfg.IndexFile) {
		t.Fatalf("openssl.cnf 缺少 database 路径：%s", conf)
	}
}

// TestLoadConfigMissingCA 校验缺 CA 时返回前置条件错误。
func TestLoadConfigMissingCA(t *testing.T) {
	root := t.TempDir()
	pki := filepath.Join(root, "pki")
	_, err := LoadConfigFrom(mapSource{"PKI_DIR": pki})
	if err == nil {
		t.Fatal("缺少 CA 证书应当报错")
	}
	if KindOf(err) != KindPrecondition {
		t.Fatalf("错误类别 = %v, want KindPrecondition（%v）", KindOf(err), err)
	}
}

// TestLoadConfigInvalidDays 校验非法整数返回用法错误。
func TestLoadConfigInvalidDays(t *testing.T) {
	root := t.TempDir()
	pki := filepath.Join(root, "pki")
	if err := os.MkdirAll(pki, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pki, "ca.crt"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfigFrom(mapSource{"PKI_DIR": pki, "DEFAULT_DAYS": "abc"})
	if err == nil || KindOf(err) != KindUsage {
		t.Fatalf("非法 DEFAULT_DAYS 应返回用法错误，实际: %v", err)
	}
}
