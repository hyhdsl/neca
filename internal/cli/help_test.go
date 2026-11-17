package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hyhdsl/neca/internal/i18n"
)

func TestSubcommandHelp(t *testing.T) {
	defer func() { _ = i18n.SetLanguage("zh-CN") }()
	for path := range helpTexts {
		args := append([]string{"--lang", "en-US"}, strings.Split(path, " ")...)
		args = append(args, "--help")
		var out, errOut bytes.Buffer
		code := RunWithIO(args, strings.NewReader(""), &out, &errOut)
		if code != ExitOK {
			t.Fatalf("%s: exit=%d stderr=%q", path, code, errOut.String())
		}
		if !strings.Contains(strings.ToLower(out.String()), "usage:") {
			t.Fatalf("%s: missing usage: %q", path, out.String())
		}
	}
}
