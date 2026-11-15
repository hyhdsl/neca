package cli

import (
	"strings"
	"testing"
)

func TestLocalizeOutput(t *testing.T) {
	input := "CN            状态    序列号                                    平台             到期                  备注\n" +
		"------------  ------  ----------------------------------------  ---------------  --------------------  ----------\n" +
		"TMP01         有效    0123456789ABCDEF  windows,android  Nov 14 13:56:54 2027\n" +
		"(证书已删除)  已吊销  DEADBEEF          -                -                     文件已删除\n" +
		"[INFO] CRL 吊销条目: 5\n" +
		"[OK]   p12 检查: TMP01 (3DES ✓, 证书 1 张, 密码 12 位)\n" +
		"[WARN] TMP01: p12 使用 PBES2/AES（MIUI 可能卡在“提取证书”）\n" +
		"[INFO] 生成密钥与证书 TMP02（30 天）...\n" +
		"[OK]   Windows 包: /tmp/Windows\n" +
		"[OK]   已签发 TMP02（序列 AABB，平台 windows,android）\n" +
		"[OK]   CRL 已更新: /tmp/ca.crl.pem\n"
	output := localizeOutput(input)
	for _, want := range []string{
		"Status", "Valid", "Revoked", "(deleted)", "file deleted",
		"CRL revoked entries: 5", "p12 check: TMP01 (3DES ✓, 1 cert(s), password 12 chars)",
		"p12 uses PBES2/AES", "Generating key and certificate TMP02 (30 days)...",
		"Windows bundle:", "Issued TMP02 (serial AABB, platforms windows,android)",
		"CRL updated:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("localized output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "证书已删除") || strings.Contains(output, "已签发") {
		t.Fatalf("localized output still contains Chinese status text:\n%s", output)
	}
}
