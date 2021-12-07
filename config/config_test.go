package config_test

import (
	"github.com/stretchr/testify/assert"
	"heckel.io/ntfy/config"
	"testing"
)

func TestConfig_New(t *testing.T) {
	c := config.New(":1234")
	assert.Equal(t, ":1234", c.ListenHTTP)
}
