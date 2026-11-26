// 本文件提供交互式确认与 --yes 处理。
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ConfirmRequest describes one interactive confirmation.
type ConfirmRequest struct {
	Action  string // 动作，例如 "trash purge"
	Subject string // 对象，例如 CN 或服务名
	Default bool   // 无法读取输入/回车时的默认值；后台命令应传 false
}

// Confirmer is the small seam used by commands that need Y/N confirmation.
// Keeping it separate lets future commands opt in or out without rewriting
// command logic, and lets tests use a deterministic fake.
type Confirmer interface {
	Confirm(ctx context.Context, req ConfirmRequest) (bool, error)
}

// TerminalConfirmer implements Confirmer for a CLI terminal.
//
// AssumeYes is the explicit automation escape hatch (--yes). It must be
// audited by the caller. Without AssumeYes, only y/yes returns true; every
// other answer (including EOF) returns the request default.
type TerminalConfirmer struct {
	In        io.Reader
	Out       io.Writer
	AssumeYes bool
}

func (c TerminalConfirmer) Confirm(ctx context.Context, req ConfirmRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if c.AssumeYes {
		return true, nil
	}
	if c.In == nil || c.Out == nil {
		return false, errors.New("cli: confirmer requires In and Out")
	}
	prompt := strings.TrimSpace(strings.Join([]string{req.Action, req.Subject}, " "))
	if prompt == "" {
		prompt = "确认执行"
	}
	if _, err := fmt.Fprintf(c.Out, "%s [y/N]: ", prompt); err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(c.In)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return req.Default, nil
	}
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "y", "yes":
		return true, nil
	default:
		return req.Default, nil
	}
}
