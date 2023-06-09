package server

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func newTestWebPushStore(t *testing.T, filename string) *webPushStore {
	webPush, err := newWebPushStore(filename)
	require.Nil(t, err)
	return webPush
}
