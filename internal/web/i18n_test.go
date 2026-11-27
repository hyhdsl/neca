// 本文件测试 i18n 相关逻辑。
package web

import (
	"reflect"
	"testing"
)

func TestWebLocaleKeysAligned(t *testing.T) {
	zh := webLocaleKeys("zh-CN")
	tw := webLocaleKeys("zh-TW")
	en := webLocaleKeys("en-US")
	if len(zh) == 0 || !reflect.DeepEqual(zh, tw) || !reflect.DeepEqual(zh, en) {
		t.Fatalf("locale keys differ: zh=%d tw=%d en=%d", len(zh), len(tw), len(en))
	}
}

func TestWebTranslator(t *testing.T) {
	zh, err := newWebTranslator("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if got := zh.T("web.nav.certs"); got != "证书" {
		t.Fatalf("zh nav = %q", got)
	}
	en, err := newWebTranslator("en-US")
	if err != nil {
		t.Fatal(err)
	}
	if got := en.T("web.nav.certs"); got != "Certificates" {
		t.Fatalf("en nav = %q", got)
	}
	if got := en.T("missing.key"); got != "missing.key" {
		t.Fatalf("fallback = %q", got)
	}
	tw, err := newWebTranslator("zh-TW")
	if err != nil {
		t.Fatal(err)
	}
	if got := tw.Language(); got != "zh-TW" {
		t.Fatalf("zh-TW language = %q", got)
	}
}
