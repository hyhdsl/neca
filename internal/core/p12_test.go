package core

import "testing"

func TestParseP12Profile(t *testing.T) {
	for _, raw := range []string{"", "compat", "modern"} {
		profile, err := ParseP12Profile(raw)
		if err != nil {
			t.Fatalf("ParseP12Profile(%q) error = %v", raw, err)
		}
		if raw == "" && profile != P12Compat {
			t.Fatalf("空值应回退 compat，实际 %q", profile)
		}
	}
	if _, err := ParseP12Profile("bad"); err == nil {
		t.Fatal("未知 profile 应报错")
	}
	if P12Compat.KeyPBE() != "PBE-SHA1-3DES" || P12Compat.MACAlg() != "sha1" {
		t.Fatal("compat 参数不正确")
	}
	if P12Modern.KeyPBE() != "AES-256-CBC" || P12Modern.MACAlg() != "sha256" {
		t.Fatal("modern 参数不正确")
	}
}
