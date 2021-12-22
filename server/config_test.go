package server_test

import (
	"github.com/stretchr/testify/assert"
	"heckel.io/ntfy/server"
	"testing"
)

func TestConfig_New(t *testing.T) {
	c := server.NewConfig()
	assert.Equal(t, ":80", c.ListenHTTP)
	assert.Equal(t, server.DefaultKeepaliveInterval, c.KeepaliveInterval)
}
