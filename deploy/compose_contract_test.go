// 本文件校验 Docker compose 的 CLI 默认入口，防止无参数再次被强制成 list/help。
package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestCLIComposeDefaultsToWizard(t *testing.T) {
	data, err := os.ReadFile("docker-compose.neca.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(data)
	if !strings.Contains(compose, `command: ["wizard"]`) {
		t.Fatalf("CLI compose command must default to wizard")
	}
	if strings.Contains(compose, `command: ["list"]`) || strings.Contains(compose, `command: ["help"]`) {
		t.Fatalf("CLI compose still forces list/help")
	}
}
