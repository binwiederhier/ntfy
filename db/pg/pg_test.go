package pg

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpen_InvalidScheme(t *testing.T) {
	_, err := Open("postgresql+psycopg2://user:pass@localhost/db")
	require.Error(t, err)
	require.Contains(t, err.Error(), `invalid database URL scheme "postgresql+psycopg2"`)
	require.Contains(t, err.Error(), "*****")
	require.NotContains(t, err.Error(), "pass")
}

func TestOpen_InvalidURL(t *testing.T) {
	_, err := Open("not a valid url\x00")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid database URL")
}

func TestCensorPassword(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "with password",
			url:      "postgres://user:secret@localhost/db",
			expected: "postgres://user:*****@localhost/db",
		},
		{
			name:     "without password",
			url:      "postgres://localhost/db",
			expected: "postgres://localhost/db",
		},
		{
			name:     "user only",
			url:      "postgres://user@localhost/db",
			expected: "postgres://user@localhost/db",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)
			require.Equal(t, tt.expected, censorPassword(u))
		})
	}
}
