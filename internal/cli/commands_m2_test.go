// 本文件测试 commands_m2 相关逻辑。
package cli

import "testing"

func TestParseFlags(t *testing.T) {
	pos, values, bools, err := parseFlags([]string{"TMP01", "--platforms", "windows,linux", "--zip", "--days=30"}, []string{"platforms", "days"}, []string{"zip"})
	if err != nil {
		t.Fatalf("parseFlags error = %v", err)
	}
	if len(pos) != 1 || pos[0] != "TMP01" {
		t.Fatalf("positionals = %v", pos)
	}
	if values["platforms"] != "windows,linux" || values["days"] != "30" || !bools["zip"] {
		t.Fatalf("values=%v bools=%v", values, bools)
	}
	if _, _, _, err := parseFlags([]string{"--nope"}, nil, nil); err == nil {
		t.Fatal("未知参数应报错")
	}
}
