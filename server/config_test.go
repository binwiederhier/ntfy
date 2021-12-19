package server_test

import (
	"github.com/stretchr/testify/assert"
	"heckel.io/ntfy/server"
	"testing"
)

func TestConfig_New(t *testing.T) {
	c := server.NewConfig(":1234")
	assert.Equal(t, ":1234", c.ListenHTTP)
}
