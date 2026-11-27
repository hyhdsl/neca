// Package i18n 加载内置语言文件，提供 CLI 文本翻译。
//
// 语言文件位于 locales/<lang>.json，随 embed 编入二进制；输出前缀
// [OK]/[INFO]/[WARN]/[ERR] 不翻译，保持下游契约（开发文档 §8.11）。
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localeFS embed.FS

const defaultLanguage = "zh-CN"

var (
	mu       sync.RWMutex
	catalogs map[string]map[string]string
	current  string
)

func init() {
	catalogs = make(map[string]map[string]string)
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		panic("i18n: " + err.Error())
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ".json")
		data, err := localeFS.ReadFile("locales/" + entry.Name())
		if err != nil {
			panic("i18n: " + err.Error())
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			panic("i18n: " + entry.Name() + ": " + err.Error())
		}
		catalogs[lang] = catalog
	}
	if _, ok := catalogs[defaultLanguage]; !ok {
		panic("i18n: missing " + defaultLanguage)
	}
	current = defaultLanguage
}

// NormalizeLanguage 把常见写法归一为 zh-CN / zh-TW / en-US。
func NormalizeLanguage(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return defaultLanguage, nil
	}
	raw = strings.ReplaceAll(raw, "_", "-")
	switch {
	case raw == "zh-tw" || strings.HasPrefix(raw, "zh-hant"):
		return "zh-TW", nil
	case raw == "zh" || strings.HasPrefix(raw, "zh-"):
		return "zh-CN", nil
	case raw == "en" || strings.HasPrefix(raw, "en-"):
		return "en-US", nil
	default:
		return "", fmt.Errorf("unsupported language: %s", raw)
	}
}

// Resolve 按 --lang > NECA_LANG > LC_ALL/LANG > zh-CN 选择语言。
func Resolve(explicit string) (string, error) {
	return ResolveWithConfig(explicit, "")
}

// ResolveWithConfig 在环境变量与系统 LANG 之间插入 JSON 配置的 language。
func ResolveWithConfig(explicit, configLang string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return NormalizeLanguage(explicit)
	}
	if value := os.Getenv("NECA_LANG"); value != "" {
		return NormalizeLanguage(value)
	}
	if strings.TrimSpace(configLang) != "" {
		return NormalizeLanguage(configLang)
	}
	for _, key := range []string{"LC_ALL", "LANG"} {
		if value := os.Getenv(key); value != "" {
			if lang, err := NormalizeLanguage(value); err == nil {
				return lang, nil
			}
		}
	}
	return defaultLanguage, nil
}

// SetLanguage 切换当前语言；非法语言返回错误。
func SetLanguage(lang string) error {
	normalized, err := NormalizeLanguage(lang)
	if err != nil {
		return err
	}
	if _, ok := catalogs[normalized]; !ok {
		return fmt.Errorf("unsupported language: %s", lang)
	}
	mu.Lock()
	current = normalized
	mu.Unlock()
	return nil
}

// Current 返回当前语言。
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T 返回翻译；缺 key 回退 zh-CN，再回退 key 本身。
func T(key string, args ...any) string {
	mu.RLock()
	lang := current
	mu.RUnlock()
	text := lookup(lang, key)
	if text == "" {
		text = lookup(defaultLanguage, key)
	}
	if text == "" {
		text = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

// Has 判断当前语言或默认语言是否有该 key。
func Has(key string) bool {
	mu.RLock()
	lang := current
	mu.RUnlock()
	return lookup(lang, key) != "" || lookup(defaultLanguage, key) != ""
}

func lookup(lang, key string) string {
	catalog := catalogs[lang]
	if catalog == nil {
		return ""
	}
	return catalog[key]
}

// Keys 返回某语言全部 key（用于 CI 一致性检查）。
func Keys(lang string) []string {
	catalog := catalogs[lang]
	keys := make([]string, 0, len(catalog))
	for key := range catalog {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Languages 返回内置语言列表。
func Languages() []string {
	langs := make([]string, 0, len(catalogs))
	for lang := range catalogs {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}
