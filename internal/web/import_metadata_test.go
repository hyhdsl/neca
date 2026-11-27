// 本文件补充 Web 证书 API 的导入元数据字段测试。
package web

import (
	"encoding/json"
	"testing"

	"github.com/hyhdsl/neca/internal/core"
)

func TestWebCertImportMetadataJSON(t *testing.T) {
	data, err := json.Marshal(core.WebCert{
		CN: "TMP01", Created: "2026-11-27",
		NotBefore: "Nov 26 00:00:00 2026 GMT", ImportSource: "p12",
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"created": "2026-11-27", "notBefore": "Nov 26 00:00:00 2026 GMT", "importSource": "p12",
	} {
		if got, _ := body[key].(string); got != want {
			t.Fatalf("%s = %q, want %q; json=%s", key, got, want, data)
		}
	}
}
