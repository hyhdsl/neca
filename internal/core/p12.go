package core

import "fmt"

// P12Profile 是 p12 导出算法配置。
type P12Profile string

const (
	// P12Compat 是默认兼容 profile：3DES + SHA1，MIUI/旧 Android 兼容。
	P12Compat P12Profile = "compat"
	// P12Modern 是新客户端 profile：AES-256 + SHA256。
	P12Modern P12Profile = "modern"
)

// ParseP12Profile 解析 --p12-profile / NECA_P12_PROFILE。
func ParseP12Profile(raw string) (P12Profile, error) {
	switch P12Profile(raw) {
	case "", P12Compat:
		return P12Compat, nil
	case P12Modern:
		return P12Modern, nil
	default:
		return "", fmt.Errorf("未知 p12 profile: %q（可选 compat / modern）", raw)
	}
}

// KeyPBE 返回 openssl pkcs12 的 -keypbe 参数。
func (p P12Profile) KeyPBE() string {
	if p == P12Modern {
		return "AES-256-CBC"
	}
	return "PBE-SHA1-3DES"
}

// CertPBE 返回 -certpbe 参数。
func (p P12Profile) CertPBE() string {
	if p == P12Modern {
		return "AES-256-CBC"
	}
	return "PBE-SHA1-3DES"
}

// MACAlg 返回 -macalg 参数。
func (p P12Profile) MACAlg() string {
	if p == P12Modern {
		return "sha256"
	}
	return "sha1"
}
