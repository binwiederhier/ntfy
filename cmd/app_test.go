package cmd

import (
	"bytes"
	"github.com/urfave/cli/v2"
	"io"
	"log"
	"os"
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
