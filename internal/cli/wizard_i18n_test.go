// 本文件测试 wizard 的入口、语言与主要菜单分类。
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hyhdsl/neca/internal/i18n"
)

func TestBareInvocationEntersWizard(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithIO(nil, strings.NewReader("0\n"), &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"neca 交互向导", "版本：", "neca help", "1) 证书管理", "2) 导入 / 导出"} {
		if !strings.Contains(text, want) {
			t.Fatalf("bare neca output missing %q: %q", want, text)
		}
	}
}

func TestEnglishWizardMenu(t *testing.T) {
	defer func() { _ = i18n.SetLanguage("zh-CN") }()
	var out, errOut bytes.Buffer
	code := RunWithIO([]string{"--lang", "en-US"}, strings.NewReader("0\n"), &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"Certificate management", "Import / export", "Trash", "Maintenance & diagnostics", "Help", "Select an action"} {
		if !strings.Contains(text, want) {
			t.Fatalf("English wizard menu missing %q: %q", want, text)
		}
	}
	if strings.Contains(text, "签发证书") || strings.Contains(text, "导入") {
		t.Fatalf("wizard menu still contains Chinese actions: %q", text)
	}
}

func TestWizardImportExportSubmenu(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithIO([]string{"--lang", "en-US"}, strings.NewReader("2\n0\n0\n"), &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"Import p12 certificate", "Import PEM certificate + private key", "Export issued CSV", "Export revoked CSV"} {
		if !strings.Contains(text, want) {
			t.Fatalf("import/export submenu missing %q: %q", want, text)
		}
	}
}

func TestWizardCertKeyImportPrompt(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithIO([]string{"--lang", "en-US"}, strings.NewReader("2\n2\nWIZKEY\n/tmp/cert.pem\n/tmp/key.pem\n\n\n"), &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"Certificate PEM path:", "Private key PEM path:", "--cert /tmp/cert.pem", "--key /tmp/key.pem"} {
		if !strings.Contains(text, want) {
			t.Fatalf("cert+key wizard output missing %q: %q", want, text)
		}
	}
}

func TestWizardCertificateSubmenu(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithIO([]string{"--lang", "en-US"}, strings.NewReader("1\n0\n0\n"), &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"Verify p12", "Generate client package", "Move to trash", "Expiry overview"} {
		if !strings.Contains(text, want) {
			t.Fatalf("certificate submenu missing %q: %q", want, text)
		}
	}
}

func TestWizardMaintenanceSubmenu(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithIO([]string{"--lang", "en-US"}, strings.NewReader("4\n0\n0\n"), &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"Sync index", "List download packages", "System diagnostics", "Web status"} {
		if !strings.Contains(text, want) {
			t.Fatalf("maintenance submenu missing %q: %q", want, text)
		}
	}
}

func TestWizardTrashSubmenu(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithIO([]string{"--lang", "en-US"}, strings.NewReader("3\n0\n0\n"), &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"List trash", "Restore entry", "Purge selected entry", "Purge expired entries"} {
		if !strings.Contains(text, want) {
			t.Fatalf("trash submenu missing %q: %q", want, text)
		}
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
