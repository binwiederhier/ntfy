package client_test

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/client"
	"net/http"
	"testing"
)

func TestWithHeader_PlainValueUnchanged(t *testing.T) {
	r, _ := http.NewRequest("POST", "http://127.0.0.1/mytopic", nil)
	require.Nil(t, client.WithTitle("Backup done: 3 files, 0 errors (100%)\twith tab")(r))
	require.Equal(t, "Backup done: 3 files, 0 errors (100%)\twith tab", r.Header.Get("X-Title"))
}

func TestWithHeader_SpecialCharsEncoded(t *testing.T) {
	r, _ := http.NewRequest("POST", "http://127.0.0.1/mytopic", nil)
	require.Nil(t, client.WithMessage("line 1\nline 2")(r))
	require.Nil(t, client.WithTitle("Grüße 🎉")(r))
	require.Equal(t, "=?utf-8?b?bGluZSAxCmxpbmUgMg==?=", r.Header.Get("X-Message"))
	require.Equal(t, "=?utf-8?b?R3LDvMOfZSDwn46J?=", r.Header.Get("X-Title"))
}
