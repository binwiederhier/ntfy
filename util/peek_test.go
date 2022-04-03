package util

import (
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
)

func TestPeak_LimitReached(t *testing.T) {
	underlying := io.NopCloser(strings.NewReader("1234567890"))
	peaked, err := Peek(underlying, 5)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, []byte("12345"), peaked.PeekedBytes)
	require.Equal(t, true, peaked.LimitReached)

	all, err := io.ReadAll(peaked)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, []byte("1234567890"), all)
	require.Equal(t, []byte("12345"), peaked.PeekedBytes)
	require.Equal(t, true, peaked.LimitReached)
}

func TestPeak_LimitNotReached(t *testing.T) {
	underlying := io.NopCloser(strings.NewReader("1234567890"))
	peaked, err := Peek(underlying, 15)
	if err != nil {
		t.Fatal(err)
	}
	all, err := io.ReadAll(peaked)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, []byte("1234567890"), all)
	require.Equal(t, []byte("1234567890"), peaked.PeekedBytes)
	require.Equal(t, false, peaked.LimitReached)
}

func TestPeak_Nil(t *testing.T) {
	peaked, err := Peek(nil, 15)
	if err != nil {
		t.Fatal(err)
	}
	all, err := io.ReadAll(peaked)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, []byte(""), all)
	require.Equal(t, []byte(""), peaked.PeekedBytes)
	require.Equal(t, false, peaked.LimitReached)
}
