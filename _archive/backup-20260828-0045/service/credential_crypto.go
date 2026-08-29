package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/tigerowo/freedom/config"
)

// EncryptCredential 使用 AES-256-GCM 加密供应商凭证（Cookie / AccessToken）。
// 返回 base64(nonce + ciphertext)。
func EncryptCredential(plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", nil
	}
	key, err := credentialKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptCredential 解密 AES-256-GCM 加密的供应商凭证。
// 空字符串原样返回；解密失败返回错误。
func DecryptCredential(encoded string) (string, error) {
	if strings.TrimSpace(encoded) == "" {
		return "", nil
	}
	key, err := credentialKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("密文长度不足")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// credentialKey 从配置中派生 32 字节 AES-256 密钥。
func credentialKey() ([]byte, error) {
	secret := config.Cfg.VendorCredentialKey
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("VENDOR_CREDENTIAL_KEY 未配置")
	}
	h := sha256.Sum256([]byte(secret))
	return h[:], nil
}
