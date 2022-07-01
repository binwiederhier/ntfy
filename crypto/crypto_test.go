package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"log"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	message := "this is a message or is it?"
	ciphertext, err := Encrypt(message, []byte("AES256Key-32Characters1234567890"))
	require.Nil(t, err)
	plaintext, err := Decrypt(ciphertext, []byte("AES256Key-32Characters1234567890"))
	require.Nil(t, err)
	log.Println(ciphertext)
	require.Equal(t, message, plaintext)
}

func TestEncryptExpectedOutputxxxxx(t *testing.T) {
	// These values are taken from https://docs.pushbullet.com/#encryption
	// The following expected ciphertext from the site was used as a baseline:
	//   MQS2K9l3G8YoLccJooY64kDeWjbkI3fAx4WcrYNtbz4p8Q==
	//   31 04b62bd9771bc6282dc709a2863ae240 de5a36e42377c0c7859cad83 6d6f3e29f1
	//   v  tag                              nonce                    ciphertext
	message := "meow!"
	key, _ := base64.StdEncoding.DecodeString("1sW28zp7CWv5TtGjlQpDHHG4Cbr9v36fG5o4f74LsKg=")
	nonce, _ := hex.DecodeString("de5a36e42377c0c7859cad83")
	ciphertext, err := encryptWithNonce(message, nonce, key)
	require.Nil(t, err)
	require.Equal(t, "MQS2K9l3G8YoLccJooY64kDeWjbkI3fAx4WcrYNtbz4p8Q==", ciphertext)
}

func TestEncryptExpectedOutput(t *testing.T) {
	// These values are taken from https://docs.pushbullet.com/#encryption, meaning that
	// all of this is compatible with how Pushbullet encrypts
	encryptedMessage := "MSfJxxY5YdjttlfUkCaKA57qU9SuCN8+ZhYg/xieI+lDnQ=="
	key, _ := base64.StdEncoding.DecodeString("1sW28zp7CWv5TtGjlQpDHHG4Cbr9v36fG5o4f74LsKg=")
	plaintext, err := Decrypt(encryptedMessage, key)
	require.Nil(t, err)
	require.Equal(t, "meow!", plaintext)
}
