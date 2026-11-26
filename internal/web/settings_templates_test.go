// 本文件测试 settings_templates 相关逻辑。
package web

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

func TestSettingsTemplatesRegistered(t *testing.T) {
	templates, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	for _, name := range []string{"settings", "settings_security", "settings_about"} {
		if templates[name] == nil {
			t.Fatalf("template %q is not registered", name)
		}
	}
}

func TestIssuePageNavAndPlatforms(t *testing.T) {
	templates, err := loadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	translator, err := newWebTranslator("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = templates["cert_new"].ExecuteTemplate(&out, "layout.html", pageData{
		Title: "签发", Nav: "issue", Lang: "zh-CN", Tr: translator, Writable: true,
		Days: 365, PlatformChoices: buildPlatformChoices(core.DefaultPlatforms()), P12Profile: "compat",
		Version: "test", Path: "/certs/new", CSRF: "test", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if !strings.Contains(html, `data-testid="nav-issue" aria-current="page"`) {
		t.Fatal("issue nav is not highlighted")
	}
	if !strings.Contains(html, `name="platform"`) || !strings.Contains(html, `value="windows"`) {
		t.Fatal("platform checkboxes are missing")
	}
	if !strings.Contains(html, `name="generateInstallers"`) {
		t.Fatal("installer opt-in checkbox is missing")
	}
}

func TestSettingsTemplatesRender(t *testing.T) {
	templates, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	translator, err := newWebTranslator("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	webConfig := &core.WebConfig{
		Enabled: true, Listen: "127.0.0.1:18080", IdleTimeoutRaw: "30m",
		AllowHighRiskSettings: true,
	}
	for _, name := range []string{"settings", "settings_security", "settings_about"} {
		var out bytes.Buffer
		err := templates[name].ExecuteTemplate(&out, "layout.html", pageData{
			Title: "设置", Nav: name, Lang: "zh-CN", Tr: translator,
			Web: webConfig, Writable: true, HelperAvailable: true,
			TrashEnabled: true, SettingsEnabled: true, TrashRetention: "不限时",
			IdleTimeoutOptions: []string{"30m"},
			Version:            "test", Path: "/settings", CSRF: "test", Now: time.Now(),
		})
		if err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		html := out.String()
		if !strings.Contains(html, "nav-settings-group") || !strings.Contains(html, "<details") || !strings.Contains(html, "nav-chevron") {
			t.Fatalf("render %s did not include collapsible settings submenu", name)
		}
		if !strings.Contains(html, "<details class=\"nav-group is-active\"") {
			t.Fatalf("render %s did not open active settings submenu", name)
		}
		if strings.Contains(html, "top-downloads") {
			t.Fatalf("render %s still includes the top download button", name)
		}
		switch name {
		case "settings_security":
			if !strings.Contains(html, "high-risk-dialog") || !strings.Contains(html, "high-risk-save") || !strings.Contains(html, "readonly-warning") {
				t.Fatal("security page is missing save dialog or read-only warning")
			}
		case "settings_about":
			if !strings.Contains(html, "github.com/hyhdsl/neca") || !strings.Contains(html, "neca.nekobiglazyer.work") || !strings.Contains(html, "AGPL-3.0-only") {
				t.Fatal("about page is missing software information")
			}
		}
	}
}
func TestSecurityReadOnlyFallback(t *testing.T) {
	templates, err := loadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	translator, err := newWebTranslator("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	webConfig := &core.WebConfig{
		Enabled: true, Listen: "127.0.0.1:18080", ReadOnly: true,
		IdleTimeoutRaw: "30m", AllowHighRiskSettings: true,
	}
	var out bytes.Buffer
	err = templates["settings_security"].ExecuteTemplate(&out, "layout.html", pageData{
		Title: "安全", Nav: "settings-security", Lang: "zh-CN", Tr: translator,
		Web: webConfig, Writable: false, HelperAvailable: true,
		SettingsEnabled: true, IdleTimeoutOptions: []string{"30m"},
		Version: "test", Path: "/settings/security", CSRF: "test", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if strings.Contains(html, "high-risk-form") {
		t.Fatal("read-only security page must not expose the edit form")
	}
	if !strings.Contains(html, "Web 当前为只读模式") {
		t.Fatal("read-only fallback explanation is missing")
	}
	if !strings.Contains(html, "是") {
		t.Fatal("read-only current value is missing")
	}
}
