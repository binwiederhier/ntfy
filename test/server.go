package test

import (
	"fmt"
	"heckel.io/ntfy/server"
	"math/rand"
	"net/http"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
}

// StartServer starts a server.Server with a random port and waits for the server to be up
func StartServer(t *testing.T) (*server.Server, int) {
	port := 10000 + rand.Intn(20000)
	conf := server.NewConfig()
	conf.ListenHTTP = fmt.Sprintf(":%d", port)
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
