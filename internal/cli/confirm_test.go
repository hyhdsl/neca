// 本文件测试 confirm 相关逻辑。
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestConfirmerYes(t *testing.T) {
	var out bytes.Buffer
	c := TerminalConfirmer{In: strings.NewReader("y\n"), Out: &out}
	ok, err := c.Confirm(context.Background(), ConfirmRequest{Action: "purge", Subject: "alice"})
	if err != nil || !ok {
		t.Fatalf("Confirm(y) = %v, %v; want true, nil", ok, err)
	}
	if !strings.Contains(out.String(), "purge alice [y/N]") {
		t.Fatalf("prompt = %q", out.String())
	}
}

func TestConfirmerDefaultNo(t *testing.T) {
	for _, input := range []string{"", "n\n", "no\n", "anything\n"} {
		var out bytes.Buffer
		c := TerminalConfirmer{In: strings.NewReader(input), Out: &out}
		ok, err := c.Confirm(context.Background(), ConfirmRequest{Action: "purge", Default: false})
		if err != nil || ok {
			t.Fatalf("Confirm(%q) = %v, %v; want false, nil", input, ok, err)
		}
	}
}

func TestConfirmerAssumeYes(t *testing.T) {
	c := TerminalConfirmer{AssumeYes: true}
	ok, err := c.Confirm(context.Background(), ConfirmRequest{Action: "lock"})
	if err != nil || !ok {
		t.Fatalf("AssumeYes = %v, %v; want true, nil", ok, err)
	}
}

func TestConfirmerCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := TerminalConfirmer{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}}
	ok, err := c.Confirm(ctx, ConfirmRequest{Action: "lock"})
	if err == nil || ok {
		t.Fatalf("canceled Confirm = %v, %v; want false, error", ok, err)
	}
}

func TestConfirmerRequiresIO(t *testing.T) {
	c := TerminalConfirmer{}
	if ok, err := c.Confirm(context.Background(), ConfirmRequest{Action: "lock"}); err == nil || ok {
		t.Fatalf("missing I/O = %v, %v; want false, error", ok, err)
	}
}

func TestRegistryMetadataDefaults(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(noopCommand("list"))
	cmd, _ := registry.Lookup("list")
	if cmd.Surface != SurfaceBackend || cmd.Confirmation != ConfirmationNone {
		t.Fatalf("defaults = %q/%q, want backend/none", cmd.Surface, cmd.Confirmation)
	}
}

func TestRegistryRejectsInvalidMetadata(t *testing.T) {
	cases := []Command{
		{Name: "bad-surface", Surface: "desktop", Run: func(Env) int { return ExitOK }},
		{Name: "bad-confirm", Confirmation: "maybe", Run: func(Env) int { return ExitOK }},
		{Name: "read-confirm", Write: false, Confirmation: ConfirmationYN, Run: func(Env) int { return ExitOK }},
	}
	for _, cmd := range cases {
		registry := NewRegistry()
		if err := registry.Register(cmd); err == nil {
			t.Fatalf("Register(%+v) 应当报错", cmd)
		}
	}
}

func TestRegistryAcceptsWriteConfirmation(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Command{Name: "purge", Write: true, Confirmation: ConfirmationYN, Run: func(Env) int { return ExitOK }}); err != nil {
		t.Fatalf("write + confirmation 应成功：%v", err)
	}
}
