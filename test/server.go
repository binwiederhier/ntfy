package test

import (
	"fmt"
	"heckel.io/ntfy/v2/server"
	"math/rand"
	"net/http"
	"path/filepath"
	"testing"
)

// StartServer starts a server.Server with a random port and waits for the server to be up
func StartServer(t *testing.T) (*server.Server, int) {
	return StartServerWithConfig(t, server.NewConfig())
}

// StartServerWithConfig starts a server.Server with a random port and waits for the server to be up
func StartServerWithConfig(t *testing.T, conf *server.Config) (*server.Server, int) {
	port := 10000 + rand.Intn(30000)
	conf.ListenHTTP = fmt.Sprintf(":%d", port)
	conf.AttachmentCacheDir = t.TempDir()
	conf.CacheFile = filepath.Join(t.TempDir(), "cache.db")
	s, err := server.New(conf)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := s.Run(); err != nil && err != http.ErrServerClosed {
			panic(err) // 'go vet' complains about 't.Fatal(err)'
		}
	}()
	WaitForPortUp(t, port)
	return s, port
}

// StopServer stops the test server and waits for the port to be down
func StopServer(t *testing.T, s *server.Server, port int) {
	s.Stop()
	WaitForPortDown(t, port)
}
