package cmd

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_Subscribe_Default_UserPass_Subscription_Token(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-user: philipp
default-password: mypass
subscribe:
  - topic: mytopic
    token: tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_Token_Subscription_UserPass(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Basic cGhpbGlwcDpteXBhc3M=", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-token: tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
subscribe:
  - topic: mytopic
    user: philipp
    password: mypass
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_Token_Subscription_Token(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-token: tk_FAKETOKEN01234567890FAKETOKEN
subscribe:
  - topic: mytopic
    token: tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_UserPass_Subscription_UserPass(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Basic cGhpbGlwcDpteXBhc3M=", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-user: fake
default-password: password
subscribe:
  - topic: mytopic
    user: philipp
    password: mypass
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_Token_Subscription_Empty(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-token: tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
subscribe:
  - topic: mytopic
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_UserPass_Subscription_Empty(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Basic cGhpbGlwcDpteXBhc3M=", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-user: philipp
default-password: mypass
subscribe:
  - topic: mytopic
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_Empty_Subscription_Token(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
subscribe:
  - topic: mytopic
    token: tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_Empty_Subscription_UserPass(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Basic cGhpbGlwcDpteXBhc3M=", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
subscribe:
  - topic: mytopic
    user: philipp
    password: mypass
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_Token_CLI_Token(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-token: tk_FAKETOKEN0123456789FAKETOKEN
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename, "--token", "tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", "mytopic"}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_Token_CLI_UserPass(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Basic cGhpbGlwcDpteXBhc3M=", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-token: tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename, "--user", "philipp:mypass", "mytopic"}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Default_Token_Subscription_Token_CLI_UserPass(t *testing.T) {
	message := `{"id":"RXIQBFaieLVr","time":124,"expires":1124,"event":"message","topic":"mytopic","message":"triggered"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic/json", r.URL.Path)
		require.Equal(t, "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(message))
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(fmt.Sprintf(`
default-host: %s
default-token: tk_FAKETOKEN01234567890FAKETOKEN
subscribe:
  - topic: mytopic
    token: tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
`, server.URL)), 0600))

	app, _, stdout, _ := newTestApp()

	require.Nil(t, app.Run([]string{"ntfy", "subscribe", "--poll", "--from-config", "--config=" + filename, "--user", "philipp:mypass"}))

	require.Equal(t, message, strings.TrimSpace(stdout.String()))
}

func TestCLI_Subscribe_Token_And_UserPass(t *testing.T) {
	app, _, _, _ := newTestApp()
	err := app.Run([]string{"ntfy", "subscribe", "--poll", "--token", "tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", "--user", "philipp:mypass", "mytopic", "triggered"})
	require.Error(t, err)
	require.Equal(t, "cannot set both --user and --token", err.Error())
}
