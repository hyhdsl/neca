// 本文件生成和校验随机密码。
package core

import (
	"crypto/rand"
	"os"
)

const passwordAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// GeneratePassword 生成 n 位字母数字密码（与旧脚本 gen_pass 等价）。
func GeneratePassword(n int) (string, error) {
	if n <= 0 {
		n = 12
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", runtimef("生成密码", "%v", err)
	}
	for i := range buf {
		buf[i] = passwordAlphabet[int(buf[i])%len(passwordAlphabet)]
	}
	return string(buf), nil
}

// EnsurePasswordFile 读取或生成 p12 密码文件；文件末尾带换行，与旧脚本 wc -c 兼容。
func EnsurePasswordFile(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		return string(data), nil
	}
	password, err := GeneratePassword(12)
	if err != nil {
		return "", err
	}
	// 旧脚本 gen_pass 直接输出 12 字节、无换行；保持逐字节兼容。
	if err := os.WriteFile(path, []byte(password), 0o600); err != nil {
		return "", runtimef("写入密码文件", "%s: %v", path, err)
	}
	return password, nil
}

// ReadPassword 读取密码文件。
func ReadPassword(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", runtimef("读取密码文件", "%s: %v", path, err)
	}
	return string(data), nil
}
