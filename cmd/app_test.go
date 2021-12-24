package cmd

import (
	"bytes"
	"encoding/json"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/client"
	"io"
	"log"
	"os"
	"strings"
	"testing"
)

// This only contains helpers so far

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func newTestApp() (*cli.App, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	var stdin, stdout, stderr bytes.Buffer
	app := New()
	app.Reader = &stdin
	app.Writer = &stdout
	app.ErrWriter = &stderr
	return app, &stdin, &stdout, &stderr
}

func toMessage(t *testing.T, s string) *client.Message {
	var m *client.Message
	if err := json.NewDecoder(strings.NewReader(s)).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}
