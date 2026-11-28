// 本文件管理 necap 主密钥与 TOTP secret 的 AES-GCM 封装。
package necap

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const masterKeySize = 32

// LoadOrCreateMasterKey 读取 32 字节主密钥；不存在时以 0600 原子创建。
func LoadOrCreateMasterKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != masterKeySize {
			return nil, fmt.Errorf("主密钥长度必须为 %d 字节", masterKeySize)
		}
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取主密钥: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建主密钥目录: %w", err)
	}
	key := make([]byte, masterKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("生成主密钥: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return LoadOrCreateMasterKey(path)
		}
		return nil, fmt.Errorf("创建主密钥: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		os.Remove(path)
		return nil, fmt.Errorf("写入主密钥: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return nil, fmt.Errorf("关闭主密钥: %w", err)
	}
	return key, nil
}

// SealTOTP 用主密钥加密用户 TOTP secret；AAD 绑定用户与因子类型。
func SealTOTP(key []byte, userID string, secret []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成 TOTP nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, secret, []byte("necap-totp:v1:"+userID)), nil
}

// OpenTOTP 解密 TOTP secret。
func OpenTOTP(key []byte, userID string, sealed []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize() {
		return nil, fmt.Errorf("TOTP ciphertext 截断")
	}
	nonce := sealed[:aead.NonceSize()]
	return aead.Open(nil, nonce, sealed[aead.NonceSize():], []byte("necap-totp:v1:"+userID))
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != masterKeySize {
		return nil, fmt.Errorf("主密钥长度必须为 %d 字节", masterKeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
