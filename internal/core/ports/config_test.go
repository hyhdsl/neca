package ports

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// staticSource 是测试用的配置来源；err 非空时模拟读取失败。
type staticSource struct {
	name   string
	values map[string]string
	err    error
}

// 编译期断言：staticSource 必须满足 Source。
var _ Source = staticSource{}

// Name 返回测试来源名。
func (s staticSource) Name() string { return s.name }

// Load 返回预设值或错误。
func (s staticSource) Load(ctx context.Context) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.values, nil
}

// TestResolverPrecedence 校验"先注册的来源优先"：
// 环境变量层应覆盖文件层，而文件层独有的 key 仍然保留。
func TestResolverPrecedence(t *testing.T) {
	env := staticSource{name: "env", values: map[string]string{
		"PKI_DIR": "/env/pki",
		"ORG":     "EnvOrg",
	}}
	file := staticSource{name: "file", values: map[string]string{
		"PKI_DIR":      "/file/pki",
		"DEFAULT_DAYS": "365",
	}}

	got, err := NewResolver(env, file).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got["PKI_DIR"] != "/env/pki" {
		t.Fatalf("PKI_DIR = %q, want /env/pki（先注册的来源优先）", got["PKI_DIR"])
	}
	if got["ORG"] != "EnvOrg" {
		t.Fatalf("ORG = %q, want EnvOrg", got["ORG"])
	}
	if got["DEFAULT_DAYS"] != "365" {
		t.Fatalf("DEFAULT_DAYS = %q, want 365（低优先级来源补齐）", got["DEFAULT_DAYS"])
	}
}

// TestResolverError 校验来源失败时立即返回错误，并带上来源名，
// 避免使用"半套配置"继续执行。
func TestResolverError(t *testing.T) {
	boom := errors.New("permission denied")
	bad := staticSource{name: "file", err: boom}

	_, err := NewResolver(bad).Resolve(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("Resolve() error = %v, want boom", err)
	}
	if !strings.Contains(err.Error(), "file") {
		t.Fatalf("错误信息应包含来源名：%v", err)
	}
}

// TestResolverSkipsNil 校验可选来源传 nil 时被忽略，
// 便于调用方按开关拼装配置层而不写额外判断。
func TestResolverSkipsNil(t *testing.T) {
	file := staticSource{name: "file", values: map[string]string{"PKI_DIR": "/pki"}}
	got, err := NewResolver(nil, file, nil).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got["PKI_DIR"] != "/pki" {
		t.Fatalf("PKI_DIR = %q, want /pki", got["PKI_DIR"])
	}
}
