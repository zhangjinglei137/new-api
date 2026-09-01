package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// secretEnvelopeVersion 标识 EncryptSecret 产出的 envelope 结构版本。
// DecryptSecret 对未知版本按历史明文原样返回（向后兼容）。
const secretEnvelopeVersion = 1

// secretEnvelope 是加密凭证的自描述 envelope。n/c 分别为 base64 编码的
// GCM nonce 与密文，v 用于区分密文与历史明文（历史明文无法解析出该结构）。
type secretEnvelope struct {
	Version int    `json:"v"`
	Nonce   string `json:"n"`
	Cipher  string `json:"c"`
}

func secretEncryptionKey() []byte {
	sum := sha256.Sum256([]byte(CryptoSecret))
	return sum[:]
}

// EncryptSecret 用 AES-256-GCM 加密任意字符串，返回 base64(JSON envelope {v,n,c})。
// v=版本(1)，n=base64 nonce(12B)，c=base64 ciphertext。密钥 = sha256(CryptoSecret)[:32]。
// CryptoSecret 为空时报错。
func EncryptSecret(plain string) (string, error) {
	if CryptoSecret == "" {
		return "", fmt.Errorf("CryptoSecret is not configured")
	}
	block, err := aes.NewCipher(secretEncryptionKey())
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
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	envelope := secretEnvelope{
		Version: secretEnvelopeVersion,
		Nonce:   base64.StdEncoding.EncodeToString(nonce),
		Cipher:  base64.StdEncoding.EncodeToString(ciphertext),
	}
	encoded, err := Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// DecryptSecret 解密 EncryptSecret 的产物。输入不是合法 envelope（解析失败 /
// 无 v 字段 / 未知版本）时按历史明文原样返回；合法则解密。空串返回空串。
func DecryptSecret(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	var envelope secretEnvelope
	if err := UnmarshalJsonStr(enc, &envelope); err != nil {
		return enc, nil
	}
	if envelope.Version != secretEnvelopeVersion {
		return enc, nil
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return "", fmt.Errorf("invalid envelope nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Cipher)
	if err != nil {
		return "", fmt.Errorf("invalid envelope ciphertext: %w", err)
	}
	block, err := aes.NewCipher(secretEncryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(nonce) != gcm.NonceSize() {
		return "", fmt.Errorf("invalid envelope nonce size")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func GenerateHMACWithKey(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateHMAC(data string) string {
	h := hmac.New(sha256.New, []byte(CryptoSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func Password2Hash(password string) (string, error) {
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	return string(hashedPassword), err
}

func ValidatePasswordAndHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
