package version

import (
	"strings"
	"testing"
)

// TestGetFillsRuntimeFields 校验 Get 会补齐编译期与运行期字段，
// 避免 String() 输出空值导致线上排查版本时失去线索。
func TestGetFillsRuntimeFields(t *testing.T) {
	info := Get()
	if info.Version == "" || info.Commit == "" || info.Date == "" {
		t.Fatalf("版本字段不应为空：%+v", info)
	}
	if info.GoVersion == "" || info.OS == "" || info.Arch == "" {
		t.Fatalf("运行时字段不应为空：%+v", info)
	}
}

// TestStringFormat 固化版本摘要的格式，方便脚本 grep 与人工核对。
func TestStringFormat(t *testing.T) {
	info := Info{
		Version:   "v0.1.0",
		Commit:    "abc1234",
		Date:      "2026-11-15T00:00:00Z",
		GoVersion: "go1.26.11",
		OS:        "linux",
		Arch:      "amd64",
	}
	got := info.String()
	want := "neca v0.1.0 (commit abc1234, built 2026-11-15T00:00:00Z, go1.26.11 linux/amd64)"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, "neca ") {
		t.Fatalf("版本摘要必须以程序名开头：%q", got)
	}
}
