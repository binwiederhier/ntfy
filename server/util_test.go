package server

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestReadBoolParam(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://ntfy.sh/mytopic?up=1&firebase=no", nil)
	up := readBoolParam(r, false, "x-up", "up")
	firebase := readBoolParam(r, true, "x-firebase", "firebase")
	require.Equal(t, true, up)
	require.Equal(t, false, firebase)

	r, _ = http.NewRequest("GET", "https://ntfy.sh/mytopic", nil)
	r.Header.Set("X-Up", "yes")
	r.Header.Set("X-Firebase", "0")
	up = readBoolParam(r, false, "x-up", "up")
	firebase = readBoolParam(r, true, "x-firebase", "firebase")
	require.Equal(t, true, up)
	require.Equal(t, false, firebase)

	r, _ = http.NewRequest("GET", "https://ntfy.sh/mytopic", nil)
	up = readBoolParam(r, false, "x-up", "up")
	firebase = readBoolParam(r, true, "x-up", "up")
	require.Equal(t, false, up)
	require.Equal(t, true, firebase)
}
