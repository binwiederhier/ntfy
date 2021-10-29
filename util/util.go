package util

import (
	"math/rand"
	"os"
	"time"
)

const (
	randomStringCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var (
	random = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func FileExists(filename string) bool {
	stat, _ := os.Stat(filename)
	return stat != nil
}

// RandomString returns a random string with a given length
func RandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = randomStringCharset[random.Intn(len(randomStringCharset))]
	}
	return string(b)
}
