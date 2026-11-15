package i18n

import (
	"reflect"
	"testing"
)

func TestLocaleKeysAligned(t *testing.T) {
	zh := Keys("zh-CN")
	en := Keys("en-US")
	if len(zh) == 0 || len(en) == 0 {
		t.Fatal("locale catalogs must not be empty")
	}
	if !reflect.DeepEqual(zh, en) {
		t.Fatalf("zh-CN/en-US keys differ:\nzh=%v\nen=%v", zh, en)
	}
}

func TestNormalizeAndSelect(t *testing.T) {
	cases := map[string]string{
		"":        "zh-CN",
		"zh":      "zh-CN",
		"zh_CN":   "zh-CN",
		"zh-Hans": "zh-CN",
		"en":      "en-US",
		"en_US":   "en-US",
	}
	for raw, want := range cases {
		got, err := NormalizeLanguage(raw)
		if err != nil || got != want {
			t.Fatalf("NormalizeLanguage(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := NormalizeLanguage("fr"); err == nil {
		t.Fatal("unsupported language should fail")
	}
}

func TestTranslateAndFallback(t *testing.T) {
	original := Current()
	defer SetLanguage(original)
	if err := SetLanguage("en-US"); err != nil {
		t.Fatal(err)
	}
	if got := T("app.usage"); got != "Usage:" {
		t.Fatalf("T(app.usage) = %q", got)
	}
	if got := T("missing.key"); got != "missing.key" {
		t.Fatalf("missing key fallback = %q", got)
	}
	if err := SetLanguage("zh-CN"); err != nil {
		t.Fatal(err)
	}
	if got := T("app.usage"); got != "用法：" {
		t.Fatalf("T(app.usage) zh = %q", got)
	}
}
