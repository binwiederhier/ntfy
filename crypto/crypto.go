package crypto

import (
	"crypto/sha256"
	"golang.org/x/crypto/pbkdf2"
	"gopkg.in/square/go-jose.v2"
)

const (
	jweEncryption = jose.A256GCM
	jweAlgorithm  = jose.DIRECT
	keyLenBytes   = 32 // 256-bit for AES-256
	keyDerivIter  = 50000
)

func DeriveKey(password string, topicURL string) []byte {
	salt := sha256.Sum256([]byte(topicURL))
	return pbkdf2.Key([]byte(password), salt[:], keyDerivIter, keyLenBytes, sha256.New)
}

func Encrypt(plaintext string, key []byte) (string, error) {
	enc, err := jose.NewEncrypter(jweEncryption, jose.Recipient{Algorithm: jweAlgorithm, Key: key}, nil)
	if err != nil {
		return "", err
	}
	jwe, err := enc.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return jwe.CompactSerialize()
}

func Decrypt(input string, key []byte) (string, error) {
	jwe, err := jose.ParseEncrypted(input)
	if err != nil {
		return "", err
	}
	out, err := jwe.Decrypt(key)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
