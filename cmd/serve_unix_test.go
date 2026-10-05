//go:build (darwin || linux || dragonfly || freebsd || netbsd || openbsd) && !noserver

package cmd

import (
	"fmt"
	"math/rand"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/model"
)

func TestCLI_Serve_SIGTERMFlushesBatchedMessages(t *testing.T) {
	// systemd stops ntfy with SIGTERM on every deploy: serve must shut down gracefully and
	// persist messages still waiting in the cache write batch instead of dying mid-batch
	port := 10000 + rand.Intn(20000)
	cacheFile := filepath.Join(t.TempDir(), "cache.db")
	configFile := newEmptyFile(t)
	done := make(chan error, 1)
	go func() {
		app, _, _, _ := newTestApp()
		done <- app.Run([]string{"ntfy", "serve", "--config=" + configFile, fmt.Sprintf("--listen-http=:%d", port),
			"--cache-file=" + cacheFile, "--cache-batch-size=100", "--cache-batch-timeout=1h"})
	}()
	url := fmt.Sprintf("http://127.0.0.1:%d/mytopic", port)
	var resp *http.Response
	var err error
	for i := 0; i < 40; i++ {
		if resp, err = http.Post(url, "text/plain", strings.NewReader("batched")); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Nil(t, err)
	require.Equal(t, 200, resp.StatusCode)

	require.Nil(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))
	select {
	case err := <-done:
		require.Nil(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not exit after SIGTERM")
	}
	cache, err := message.NewSQLiteStore(cacheFile, "", time.Hour, 0, 0, false)
	require.Nil(t, err)
	defer cache.Close()
	messages, err := cache.Messages("mytopic", model.SinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 1, len(messages))
}
