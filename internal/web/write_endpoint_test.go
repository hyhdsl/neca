// 本文件测试 write_endpoint 相关逻辑。
package web

import "testing"

func TestHighRiskSettingsIsWriteEndpoint(t *testing.T) {
	if !isWriteEndpoint("/settings/high-risk") {
		t.Fatal("/settings/high-risk must pass readOnlyGuard to reach handler checks")
	}
}

func TestCertTrashIsWriteEndpoint(t *testing.T) {
	for _, path := range []string{
		"/certs/TMP01/trash",
		"/api/v1/certs/TMP01/trash",
	} {
		if !isWriteEndpoint(path) {
			t.Errorf("%s must pass readOnlyGuard to reach handler checks", path)
		}
	}
}
