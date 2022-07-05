package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)
import "gopkg.in/square/go-jose.v2"

const (
	versionByte  = 0x31 // "1"
	gcmTagSize   = 16
	gcmNonceSize = 12
)

var (
	errCiphertextTooShort          = errors.New("ciphertext too short")
	errCiphertextUnexpectedVersion = errors.New("unsupported ciphertext version")
)

// Encrypt encrypts the given plaintext with the given key using AES-GCM,
// and encodes the (version, tag, nonce, ciphertext) set as base64.
//
// The output format is (|| means concatenate):
//    "1" || tag (128 bits) || IV/nonce (96 bits) || ciphertext (remaining)
//
// This format is compatible with Pushbullet's encryption format.
// See https://docs.pushbullet.com/#encryption for details.
func Encrypt(plaintext string, key []byte) (string, error) {
	nonce := make([]byte, gcmNonceSize) // Never use more than 2^32 random nonces
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return encryptWithNonce(plaintext, nonce, key)
}

func encryptWithNonce(plaintext string, nonce, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ciphertextWithTag := aesgcm.Seal(nil, nonce, []byte(plaintext), nil)
	tagIndex := len(ciphertextWithTag) - gcmTagSize
	ciphertext, tag := ciphertextWithTag[:tagIndex], ciphertextWithTag[tagIndex:]
	output := appendSlices([]byte{versionByte}, tag, nonce, ciphertext)
	return base64.StdEncoding.EncodeToString(output), nil
}

// Decrypt decodes and decrypts a message that was encrypted with the Encrypt function.
func Decrypt(input string, key []byte) (string, error) {
	inputBytes, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return "", err
	}
	if len(inputBytes) < 1+gcmTagSize+gcmNonceSize {
		return "", errCiphertextTooShort
	}
	version, tag, nonce, ciphertext := inputBytes[0], inputBytes[1:gcmTagSize+1], inputBytes[1+gcmTagSize:1+gcmTagSize+gcmNonceSize], inputBytes[1+gcmTagSize+gcmNonceSize:]
	if version != versionByte {
		return "", errCiphertextUnexpectedVersion
	}
	cipherTextWithTag := append(ciphertext, tag...)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := aesgcm.Open(nil, nonce, cipherTextWithTag, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func EncryptJWE(plaintext string, key []byte) (string, error) {
	enc, err := jose.NewEncrypter(jose.A256GCM, jose.Recipient{Algorithm: jose.DIRECT, Key: key}, nil)
	if err != nil {
		return "", err
	}
	jwe, err := enc.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return jwe.CompactSerialize()
}

func DecryptJWE(input string, key []byte) (string, error) {
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

func appendSlices(s ...[]byte) []byte {
	var output []byte
	for _, r := range s {
		output = append(output, r...)
	}
	return output
}
