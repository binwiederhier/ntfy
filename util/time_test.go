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

func TestNextOccurrenceUTC_NextDate(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.Nil(t, err)

	timeOfDay := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC) // Run at midnight UTC
	nowInFairfieldCT := time.Date(2023, time.January, 10, 22, 19, 12, 0, loc)
	nextRunTme := NextOccurrenceUTC(timeOfDay, nowInFairfieldCT)
	require.Equal(t, time.Date(2023, time.January, 12, 0, 0, 0, 0, time.UTC), nextRunTme)
}

func TestNextOccurrenceUTC_SameDay(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.Nil(t, err)

	timeOfDay := time.Date(0, 0, 0, 4, 0, 0, 0, time.UTC) // Run at 4am UTC
	nowInFairfieldCT := time.Date(2023, time.January, 10, 22, 19, 12, 0, loc)
	nextRunTme := NextOccurrenceUTC(timeOfDay, nowInFairfieldCT)
	require.Equal(t, time.Date(2023, time.January, 11, 4, 0, 0, 0, time.UTC), nextRunTme)
}

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

func TestParseDuration(t *testing.T) {
	d, err := ParseDuration("2d")
	require.Nil(t, err)
	require.Equal(t, 48*time.Hour, d)

	d, err = ParseDuration("2h")
	require.Nil(t, err)
	require.Equal(t, 2*time.Hour, d)

	d, err = ParseDuration("0")
	require.Nil(t, err)
	require.Equal(t, time.Duration(0), d)
}

func TestFormatDuration(t *testing.T) {
	values := []struct {
		duration time.Duration
		expected string
	}{
		{24 * time.Second, "24s"},
		{56 * time.Minute, "56m"},
		{time.Hour, "1h"},
		{2 * time.Hour, "2h"},
		{24 * time.Hour, "1d"},
		{3 * 24 * time.Hour, "3d"},
	}
	for _, value := range values {
		require.Equal(t, value.expected, FormatDuration(value.duration))
		d, err := ParseDuration(FormatDuration(value.duration))
		require.Nil(t, err)
		require.Equalf(t, value.duration, d, "duration does not match: %v != %v", value.duration, d)
	}
}

func TestFormatDuration_Rounded(t *testing.T) {
	require.Equal(t, "1d", FormatDuration(47*time.Hour))
}
