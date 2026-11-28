// 本文件实现 RFC 6238 TOTP（SHA1/SHA256/SHA512）与 provisioning URI。
package necap

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base32"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTOTPDigits = 6
	DefaultTOTPPeriod = 30
	DefaultTOTPSkew   = 1
)

// GenerateTOTP 返回指定时间窗口的 TOTP 与 counter。
func GenerateTOTP(secret []byte, at time.Time, digits, period int, algorithm string) (string, uint64, error) {
	if len(secret) == 0 || period <= 0 || digits < 6 || digits > 8 {
		return "", 0, fmt.Errorf("TOTP 参数非法")
	}
	counter := uint64(at.Unix() / int64(period))
	hashFunc, err := totpHash(algorithm)
	if err != nil {
		return "", 0, err
	}
	return hotp(secret, counter, digits, hashFunc), counter, nil
}

// ValidateTOTP 校验 code，允许 ±skew 个时间窗口，返回匹配的 counter。
func ValidateTOTP(secret []byte, code string, at time.Time, digits, period, skew int, algorithm string) (uint64, bool) {
	if len(secret) == 0 || period <= 0 || digits < 6 || digits > 8 || skew < 0 {
		return 0, false
	}
	code = strings.TrimSpace(code)
	if len(code) != digits {
		return 0, false
	}
	hashFunc, err := totpHash(algorithm)
	if err != nil {
		return 0, false
	}
	base := uint64(at.Unix() / int64(period))
	for offset := -skew; offset <= skew; offset++ {
		candidate := int64(base) + int64(offset)
		if candidate < 0 {
			continue
		}
		expected := hotp(secret, uint64(candidate), digits, hashFunc)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return uint64(candidate), true
		}
	}
	return 0, false
}

func hotp(secret []byte, counter uint64, digits int, hashFunc func() hash.Hash) string {
	mac := hmac.New(hashFunc, secret)
	var buf [8]byte
	for i := 7; i >= 0; i-- {
		buf[i] = byte(counter)
		counter >>= 8
	}
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binary := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, binary%mod)
}

func totpHash(algorithm string) (func() hash.Hash, error) {
	switch strings.ToUpper(strings.TrimSpace(algorithm)) {
	case "", "SHA1":
		return sha1.New, nil
	case "SHA256":
		return sha256.New, nil
	case "SHA512":
		return sha512.New, nil
	default:
		return nil, fmt.Errorf("不支持的 TOTP 算法: %s", algorithm)
	}
}

// NewTOTPSecret 生成 160-bit 随机 secret（Base32 无填充）。
func NewTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// TOTPProvisioningURI 生成 otpauth URI；调用方不得把它写入日志。
func TOTPProvisioningURI(issuer, account, secret string, digits, period int) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	values := url.Values{}
	values.Set("secret", secret)
	values.Set("issuer", issuer)
	values.Set("algorithm", "SHA1")
	values.Set("digits", strconv.Itoa(digits))
	values.Set("period", strconv.Itoa(period))
	return "otpauth://totp/" + label + "?" + values.Encode()
}
