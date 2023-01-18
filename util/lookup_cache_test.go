package util

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLookupCache_Success(t *testing.T) {
	values, i := []string{"first", "second"}, 0
	c := NewLookupCache[string](func() (string, error) {
		time.Sleep(300 * time.Millisecond)
		v := values[i]
		i++
		return v, nil
	}, 500*time.Millisecond)

	start := time.Now()
	v, err := c.Value()
	require.Nil(t, err)
	require.Equal(t, values[0], v)
	require.True(t, time.Since(start) >= 300*time.Millisecond)

	start = time.Now()
	v, err = c.Value()
	require.Nil(t, err)
	require.Equal(t, values[0], v)
	require.True(t, time.Since(start) < 200*time.Millisecond)

	time.Sleep(550 * time.Millisecond)

	start = time.Now()
	v, err = c.Value()
	require.Nil(t, err)
	require.Equal(t, values[1], v)
	require.True(t, time.Since(start) >= 300*time.Millisecond)

	start = time.Now()
	v, err = c.Value()
	require.Nil(t, err)
	require.Equal(t, values[1], v)
	require.True(t, time.Since(start) < 200*time.Millisecond)
}

func TestLookupCache_Error(t *testing.T) {
	c := NewLookupCache[string](func() (string, error) {
		time.Sleep(200 * time.Millisecond)
		return "", errors.New("some error")
	}, 500*time.Millisecond)

	start := time.Now()
	v, err := c.Value()
	require.NotNil(t, err)
	require.Equal(t, "", v)
	require.True(t, time.Since(start) >= 200*time.Millisecond)

	start = time.Now()
	v, err = c.Value()
	require.NotNil(t, err)
	require.Equal(t, "", v)
	require.True(t, time.Since(start) >= 200*time.Millisecond)
}
