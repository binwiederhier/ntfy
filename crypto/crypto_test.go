package crypto

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	message := "this is a message or is it?"
	ciphertext, err := Encrypt(message, []byte("AES256Key-32Characters1234567890"))
	require.Nil(t, err)
	plaintext, err := Decrypt(ciphertext, []byte("AES256Key-32Characters1234567890"))
	require.Nil(t, err)
	require.Equal(t, message, plaintext)
}

func TestEncryptDecrypt_FromPHP(t *testing.T) {
	ciphertext := "eyJhbGciOiJkaXIiLCJlbmMiOiJBMjU2R0NNIn0..vbe1Qv_-mKYbUgce.EfmOUIUi7lxXZG_o4bqXZ9pmpr1Rzs4Y5QLE2XD2_aw_SQ.y2hadrN5b2LEw7_PJHhbcA"
	key := DeriveKey("secr3t password", "https://ntfy.sh/mysecret")
	plaintext, err := Decrypt(ciphertext, key)
	require.Nil(t, err)
	require.Equal(t, `{"message":"Secret!","priority":5}`, plaintext)
}

func TestEncryptDecrypt_FromPython(t *testing.T) {
	ciphertext := "eyJhbGciOiJkaXIiLCJlbmMiOiJBMjU2R0NNIn0..gSRYZeX6eBhlj13w.LOchcxFXwALXE2GqdoSwFJEXdMyEbLfLKV9geXr17WrAN-nH7ya1VQ_Y6ebT1w.2eyLaTUfc_rpKaZr4-5I1Q"
	key := DeriveKey("secr3t password", "https://ntfy.sh/mysecret")
	plaintext, err := Decrypt(ciphertext, key)
	require.Nil(t, err)
	require.Equal(t, `{"message":"Python says hi","tags":["secret"]}`, plaintext)
}
