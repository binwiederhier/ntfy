package util

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

var (
	// 2021-12-10 10:17:23 (Friday)
	base = time.Date(2021, 12, 10, 10, 17, 23, 0, time.UTC)
)

func TestParseFutureTime_11am_FutureTime(t *testing.T) {
	d, err := ParseFutureTime("11am", base)
	require.Nil(t, err)
	require.Equal(t, time.Date(2021, 12, 10, 11, 0, 0, 0, time.UTC), d) // Same day
}

func TestParseFutureTime_9am_PastTime(t *testing.T) {
	d, err := ParseFutureTime("9am", base)
	require.Nil(t, err)
	require.Equal(t, time.Date(2021, 12, 11, 9, 0, 0, 0, time.UTC), d) // Next day
}

func TestParseFutureTime_Monday_10_30pm_FutureTime(t *testing.T) {
	d, err := ParseFutureTime("Monday, 10:30pm", base)
	require.Nil(t, err)
	require.Equal(t, time.Date(2021, 12, 13, 22, 30, 0, 0, time.UTC), d)
}

func TestParseFutureTime_30m(t *testing.T) {
	d, err := ParseFutureTime("30m", base)
	require.Nil(t, err)
	require.Equal(t, time.Date(2021, 12, 10, 10, 47, 23, 0, time.UTC), d)
}

func TestParseFutureTime_30min(t *testing.T) {
	d, err := ParseFutureTime("30min", base)
	require.Nil(t, err)
	require.Equal(t, time.Date(2021, 12, 10, 10, 47, 23, 0, time.UTC), d)
}

func TestParseFutureTime_3h(t *testing.T) {
	d, err := ParseFutureTime("3h", base)
	require.Nil(t, err)
	require.Equal(t, time.Date(2021, 12, 10, 13, 17, 23, 0, time.UTC), d)
}

func TestParseFutureTime_1day(t *testing.T) {
	d, err := ParseFutureTime("1 day", base)
	require.Nil(t, err)
	require.Equal(t, time.Date(2021, 12, 11, 10, 17, 23, 0, time.UTC), d)
}

func TestParseFutureTime_UnixTime(t *testing.T) {
	d, err := ParseFutureTime("1639183911", base)
	require.Nil(t, err)
	require.Equal(t, time.Date(2021, 12, 11, 0, 51, 51, 0, time.UTC), d)
}
