// 本文件测试 wizard_i18n 相关逻辑。
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hyhdsl/neca/internal/i18n"
)

func TestEnglishWizardMenu(t *testing.T) {
	defer func() { _ = i18n.SetLanguage("zh-CN") }()
	var out, errOut bytes.Buffer
	code := RunWithIO([]string{"--lang", "en-US"}, strings.NewReader("0\n"), &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	text := out.String()
	if !strings.Contains(text, "1) Issue certificate") || !strings.Contains(text, "10) Move to trash") {
		t.Fatalf("English wizard menu missing: %q", text)
	}
	if strings.Contains(text, "签发证书") || strings.Contains(text, "删除证书") {
		t.Fatalf("wizard menu still contains Chinese actions: %q", text)
	}
}

func TestEnglishHelpAndNamespaceHelp(t *testing.T) {
	defer func() { _ = i18n.SetLanguage("zh-CN") }()
	for _, args := range [][]string{{"--lang", "en-US", "help"}, {"--lang", "en-US", "cert", "--help"}, {"--lang", "en-US", "web", "--help"}} {
		var out, errOut bytes.Buffer
		code := RunWithIO(args, strings.NewReader(""), &out, &errOut)
		if code != ExitOK {
			t.Fatalf("%v: exit = %d, stderr = %q", args, code, errOut.String())
		}
		text := out.String()
		if strings.Contains(text, "证书管理（") || strings.Contains(text, "未知 ") || strings.Contains(text, "用法:") {
			t.Fatalf("%v: untranslated help: %q", args, text)
		}
	}
}
