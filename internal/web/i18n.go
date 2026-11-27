// 本文件加载 Web 语言包。
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

//go:embed locales/*.json
var webLocaleFS embed.FS

const webDefaultLanguage = "zh-CN"

// webTranslator 为 Web 页面预留多语言支持；模板可通过 .T 使用。
//
// 当前模板仍以中文为主，翻译文件已按页面 key 准备；后续可逐项迁移并增加
// 每用户语言选择，不需要改配置 schema。
type webTranslator struct {
	lang     string
	catalogs map[string]map[string]string
}

func newWebTranslator(language string) (*webTranslator, error) {
	entries, err := webLocaleFS.ReadDir("locales")
	if err != nil {
		return nil, err
	}
	catalogs := map[string]map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ".json")
		data, err := webLocaleFS.ReadFile(path.Join("locales", entry.Name()))
		if err != nil {
			return nil, err
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			return nil, fmt.Errorf("web locale %s: %w", lang, err)
		}
		catalogs[lang] = catalog
	}
	if _, ok := catalogs[webDefaultLanguage]; !ok {
		return nil, fmt.Errorf("web locale %s missing", webDefaultLanguage)
	}
	lang := normalizeWebLanguage(language)
	if _, ok := catalogs[lang]; !ok {
		lang = webDefaultLanguage
	}
	return &webTranslator{lang: lang, catalogs: catalogs}, nil
}

func normalizeWebLanguage(raw string) string {
	value := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), "_", "-"))
	switch {
	case value == "zh-tw" || strings.HasPrefix(value, "zh-hant"):
		return "zh-TW"
	case value == "zh" || strings.HasPrefix(value, "zh-"):
		return "zh-CN"
	case value == "en" || strings.HasPrefix(value, "en-"):
		return "en-US"
	default:
		return webDefaultLanguage
	}
}

func (t *webTranslator) Language() string {
	if t == nil {
		return webDefaultLanguage
	}
	return t.lang
}

func (t *webTranslator) T(key string, args ...any) string {
	if t == nil {
		return key
	}
	text := t.catalogs[t.lang][key]
	if text == "" {
		text = t.catalogs[webDefaultLanguage][key]
	}
	if text == "" {
		text = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

func webLocaleKeys(lang string) []string {
	data, err := webLocaleFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return nil
	}
	var catalog map[string]string
	if json.Unmarshal(data, &catalog) != nil {
		return nil
	}
	keys := make([]string, 0, len(catalog))
	for key := range catalog {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
