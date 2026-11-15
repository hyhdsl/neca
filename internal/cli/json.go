package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

// jsonEnvelope 是开发文档 §6.4 的统一 JSON 信封。
type jsonEnvelope struct {
	OK         bool       `json:"ok"`
	Action     string     `json:"action,omitempty"`
	CN         string     `json:"cn,omitempty"`
	Warnings   []string   `json:"warnings,omitempty"`
	Data       any        `json:"data,omitempty"`
	Error      *jsonError `json:"error,omitempty"`
	DurationMS int64      `json:"durationMs"`
}

type jsonError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// emitJSON 输出成功信封。
func emitJSON(env Env, action, cn string, warnings []string, data any) int {
	envl := jsonEnvelope{
		OK: true, Action: action, CN: cn, Warnings: warnings, Data: data,
		DurationMS: elapsedMS(env),
	}
	writeEnvelope(env, envl)
	return ExitOK
}

// emitJSONError 输出失败信封并返回映射后的退出码。
func emitJSONError(env Env, err error) int {
	code := "runtime"
	switch core.KindOf(err) {
	case core.KindUsage:
		code = "usage"
	case core.KindPrecondition:
		code = "precondition"
	}
	envl := jsonEnvelope{
		OK: false, Action: env.Action, CN: env.CN,
		Error:      &jsonError{Code: code, Message: err.Error()},
		DurationMS: elapsedMS(env),
	}
	writeEnvelope(env, envl)
	return exitForError(err)
}

// emitUsageJSONError 为参数解析错误输出 usage 信封。
func emitUsageJSONError(env Env, err error) int {
	envl := jsonEnvelope{
		OK: false, Action: env.Action, CN: env.CN,
		Error:      &jsonError{Code: "usage", Message: err.Error()},
		DurationMS: elapsedMS(env),
	}
	writeEnvelope(env, envl)
	return ExitUsage
}

func writeEnvelope(env Env, envl jsonEnvelope) {
	data, err := json.Marshal(envl)
	if err != nil {
		fmt.Fprintf(env.Stderr, "[ERR]  %v\n", err)
		return
	}
	fmt.Fprintln(env.Stdout, string(data))
}

func elapsedMS(env Env) int64 {
	if env.Started.IsZero() {
		return 0
	}
	return time.Since(env.Started).Milliseconds()
}

// runJSONCommand 在 --json 模式下捕获人类输出；若命令自己写了合法 JSON
// 就原样透传，否则包装成信封，保证 stdout 始终是单个 JSON 对象。
func runJSONCommand(env Env, cmd *Command, stdout, stderr io.Writer) int {
	var out, errOut bytes.Buffer
	env.Stdout = &out
	env.Stderr = &errOut
	code := cmd.Run(env)

	trimmed := bytes.TrimSpace(out.Bytes())
	if len(trimmed) > 0 && json.Valid(trimmed) {
		stdout.Write(out.Bytes())
		if errOut.Len() > 0 {
			stderr.Write(errOut.Bytes())
		}
		return code
	}

	warnings := nonEmptyLines(errOut.String())
	data := map[string]any{
		"output": nonEmptyLines(out.String()),
		"stderr": warnings,
	}
	envl := jsonEnvelope{
		OK: code == 0, Action: env.Action, CN: env.CN, Warnings: warnings,
		Data: data, DurationMS: elapsedMS(env),
	}
	if code != 0 {
		envl.Error = &jsonError{Code: "command_failed", Message: fmt.Sprintf("command exited with code %d", code)}
	}
	writeEnvelope(env, envl)
	return code
}

func nonEmptyLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(text, "\r\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
