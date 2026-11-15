package web

import (
	"fmt"

	"github.com/hyhdsl/neca/internal/core"
)

func coreUsage(op, format string, args ...any) error {
	return &core.Error{Kind: core.KindUsage, Op: op, Err: fmt.Errorf(format, args...)}
}

func corePrecondition(op, format string, args ...any) error {
	return &core.Error{Kind: core.KindPrecondition, Op: op, Err: fmt.Errorf(format, args...)}
}
