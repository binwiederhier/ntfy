package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseFutureTime_TimezoneDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	for _, test := range []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"spring", time.Date(2026, 3, 7, 10, 0, 0, 0, location), time.Date(2026, 3, 8, 14, 0, 0, 0, time.UTC)},
		{"fall", time.Date(2026, 10, 31, 10, 0, 0, 0, location), time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseFutureTime("tomorrow 10am", test.now)
			require.NoError(t, err)
			require.True(t, test.want.Equal(got), "want %s, got %s", test.want, got)
			got, err = ParseFutureTime("24h", test.now)
			require.NoError(t, err)
			require.Equal(t, 24*time.Hour, got.Sub(test.now))
		})
	}
}
