package cmd

import (
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
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

func TestNewYamlSourceIncludesFlagPrecedence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "server.yml")
	require.NoError(t, os.WriteFile(file, []byte("include: local.yml\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "local.yml"), []byte("listen-http: ':9090'\n"), 0600))
	for _, tc := range []struct {
		name, env, flag, want string
	}{
		{"included config", "", "", ":9090"},
		{"environment", ":8081", "", ":8081"},
		{"command line", ":8081", ":8082", ":8082"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NTFY_TEST_CONFIG_LISTEN", tc.env)
			if tc.env == "" {
				require.NoError(t, os.Unsetenv("NTFY_TEST_CONFIG_LISTEN"))
			}
			flags := []cli.Flag{
				&cli.StringFlag{Name: "config"},
				altsrc.NewStringFlag(&cli.StringFlag{Name: "listen-http", EnvVars: []string{"NTFY_TEST_CONFIG_LISTEN"}}),
			}
			app := &cli.App{
				Flags:  flags,
				Before: initConfigFileInputSourceFunc("config", flags, nil),
				Action: func(ctx *cli.Context) error {
					require.Equal(t, tc.want, ctx.String("listen-http"))
					return nil
				},
			}
			args := []string{"ntfy", "--config", file}
			if tc.flag != "" {
				args = append(args, "--listen-http", tc.flag)
			}
			require.NoError(t, app.Run(args))
		})
	}
}

func TestNewYamlSourceIncludes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "conf"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.yml"), []byte("listen_http: ':80'\ninclude: [conf/first.yml, second.yml]\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conf/first.yml"), []byte("listen-http: ':8080'\ninclude: nested.yml\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conf/nested.yml"), []byte("cache-duration: '24h'\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.yml"), []byte("listen_http: ':9090'\n"), 0600))
	source, err := newYamlSourceFromFile(filepath.Join(dir, "server.yml"), flagsServe)
	require.NoError(t, err)
	listen, err := source.String("listen-http")
	require.NoError(t, err)
	require.Equal(t, ":9090", listen)
	cache, err := source.String("cache-duration")
	require.NoError(t, err)
	require.Equal(t, "24h", cache)
}

func TestNewYamlSourceInvalidIncludes(t *testing.T) {
	for _, value := range []string{"server.yml", "missing.yml", "''", "42", "[nested.yml, 42]"} {
		t.Run(value, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "server.yml")
			require.NoError(t, os.WriteFile(file, []byte("include: "+value+"\n"), 0600))
			_, err := newYamlSourceFromFile(file, flagsServe)
			require.Error(t, err)
		})
	}
}
