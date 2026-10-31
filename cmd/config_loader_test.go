package cmd

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestNewYamlSourceFromFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "server.yml")
	contents := `
# Normal options
listen-https: ":10443"

# Note the underscore!
listen_http: ":1080"

# OMG this is allowed now ...
K: /some/file.pem
`
	require.Nil(t, os.WriteFile(filename, []byte(contents), 0600))

	ctx, err := newYamlSourceFromFile(filename, flagsServe)
	require.Nil(t, err)

	listenHTTPS, err := ctx.String("listen-https")
	require.Nil(t, err)
	require.Equal(t, ":10443", listenHTTPS)

	listenHTTP, err := ctx.String("listen-http") // No underscore!
	require.Nil(t, err)
	require.Equal(t, ":1080", listenHTTP)

	keyFile, err := ctx.String("key-file") // Long option!
	require.Nil(t, err)
	require.Equal(t, "/some/file.pem", keyFile)
}
