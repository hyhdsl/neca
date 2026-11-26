// 本文件测试 errs 相关逻辑。
package core

import (
	"errors"
	"testing"
)

// TestKindOf 校验错误类别识别与默认值。
func TestKindOf(t *testing.T) {
	if got := KindOf(runtimef("op", "boom")); got != KindRuntime {
		t.Fatalf("runtimef = %v", got)
	}
	if got := KindOf(usagef("op", "boom")); got != KindUsage {
		t.Fatalf("usagef = %v", got)
	}
	if got := KindOf(preconditionf("op", "boom")); got != KindPrecondition {
		t.Fatalf("preconditionf = %v", got)
	}
	if got := KindOf(errors.New("plain")); got != KindRuntime {
		t.Fatalf("普通错误应默认运行期，实际 %v", got)
	}
}

// TestErrorUnwrap 校验错误链可被 errors.Is 识别。
func TestErrorUnwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	err := &Error{Kind: KindRuntime, Op: "op", Err: sentinel}
	if !errors.Is(err, sentinel) {
		t.Fatal("Error 应实现 Unwrap")
	}
}
