package common

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptSecretRoundTrip(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = "secret-crypto-roundtrip"
	t.Cleanup(func() { CryptoSecret = previous })

	for _, plain := range []string{
		"session=abc123; Path=/; HttpOnly",
		"短中文凭证值",
		"x",
		"",
	} {
		enc, err := EncryptSecret(plain)
		require.NoError(t, err)
		require.NotEmpty(t, enc)
		if plain != "" {
			assert.NotEqual(t, plain, enc)
		}
		dec, err := DecryptSecret(enc)
		require.NoError(t, err)
		assert.Equal(t, plain, dec)
	}
}

func TestEncryptSecretProducesSelfDescribingEnvelope(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = "secret-crypto-envelope"
	t.Cleanup(func() { CryptoSecret = previous })

	enc, err := EncryptSecret("token-value")
	require.NoError(t, err)

	var envelope secretEnvelope
	require.NoError(t, UnmarshalJsonStr(enc, &envelope))
	assert.Equal(t, secretEnvelopeVersion, envelope.Version)
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	require.NoError(t, err)
	assert.Equal(t, 12, len(nonce))
	_, err = base64.StdEncoding.DecodeString(envelope.Cipher)
	require.NoError(t, err)
}

func TestDecryptSecretRejectsTamperedCiphertext(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = "secret-crypto-tamper"
	t.Cleanup(func() { CryptoSecret = previous })

	enc, err := EncryptSecret("precious-cookie")
	require.NoError(t, err)

	var envelope secretEnvelope
	require.NoError(t, UnmarshalJsonStr(enc, &envelope))
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Cipher)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)
	ciphertext[0] ^= 0x01
	envelope.Cipher = base64.StdEncoding.EncodeToString(ciphertext)
	tampered, err := Marshal(envelope)
	require.NoError(t, err)

	_, err = DecryptSecret(string(tampered))
	require.Error(t, err)
}

func TestDecryptSecretUnknownVersionFallsBackToPlaintext(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = "secret-crypto-version"
	t.Cleanup(func() { CryptoSecret = previous })

	// 未知版本按历史明文原样返回（向后兼容）。
	unknownVersion := `{"v":2,"n":"YWJj","c":"ZGVm"}`
	dec, err := DecryptSecret(unknownVersion)
	require.NoError(t, err)
	assert.Equal(t, unknownVersion, dec)
}

func TestDecryptSecretNonEnvelopePlaintextPassesThrough(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = "secret-crypto-plaintext"
	t.Cleanup(func() { CryptoSecret = previous })

	for _, plain := range []string{
		"session=abc123",
		"plain-cookie",
		"12345",
		"not-json-at-all",
		"",
	} {
		dec, err := DecryptSecret(plain)
		require.NoError(t, err)
		assert.Equal(t, plain, dec)
	}
}

func TestEncryptSecretRequiresConfiguredCryptoSecret(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = ""
	t.Cleanup(func() { CryptoSecret = previous })

	_, err := EncryptSecret("anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CryptoSecret")
}

func TestDecryptSecretEmptyInputReturnsEmpty(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = "secret-crypto-empty"
	t.Cleanup(func() { CryptoSecret = previous })

	dec, err := DecryptSecret("")
	require.NoError(t, err)
	assert.Equal(t, "", dec)
}

func TestDecryptSecretInvalidEnvelopeNonceErrors(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = "secret-crypto-badnonce"
	t.Cleanup(func() { CryptoSecret = previous })

	enc, err := EncryptSecret("value")
	require.NoError(t, err)
	var envelope secretEnvelope
	require.NoError(t, UnmarshalJsonStr(enc, &envelope))
	envelope.Nonce = strings.Repeat("A", 7) // 无效 base64
	bad, err := Marshal(envelope)
	require.NoError(t, err)

	_, err = DecryptSecret(string(bad))
	require.Error(t, err)
}
