package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServer_MessageTemplate_TooLarge(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		response := request(t, s, "PUT", "/mytopic", `{"foo":"bar"}`, map[string]string{
			"X-Message":  "{{.foo}}" + strings.Repeat("x", 33*1024),
			"X-Template": "1",
		})
		require.Equal(t, 400, response.Code)
		require.Equal(t, 40056, toHTTPError(t, response.Body.String()).Code)
	})
}

func TestServer_MessageTemplate_PrintfWidthTooLarge(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		// A handful of 1MB-wide verbs would allocate several MB inside a single fmt call, where
		// the executor's context is never checked; the printf guard must reject the call before
		// fmt runs, not after the limit writer sees the output
		response := request(t, s, "PUT", "/mytopic", `{"n":1}`, map[string]string{
			"X-Message":  `{{printf "%1000000d%1000000d%1000000d" .n .n .n}}`,
			"X-Template": "1",
		})
		require.Equal(t, 400, response.Code)
		require.Equal(t, 40045, toHTTPError(t, response.Body.String()).Code)
		require.Contains(t, response.Body.String(), "printf width or precision too large")
	})
}

func TestServer_MessageTemplate_PrintfWidthTooLarge_DynamicFormat(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		// The format string is assembled at execution time, so the guard must inspect the actual
		// argument, not the template source
		response := request(t, s, "PUT", "/mytopic", `{"n":1}`, map[string]string{
			"X-Message":  `{{$f := print "%" "999999" "d" "%" "999999" "d"}}{{printf $f .n .n}}`,
			"X-Template": "1",
		})
		require.Equal(t, 400, response.Code)
		require.Contains(t, response.Body.String(), "printf width or precision too large")
	})
}

func TestServer_MessageTemplate_PrintfStarWidthTooLarge(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		// Star widths take the width from an argument; sprig's math functions (int64 results)
		// make large integer arguments reachable from a template
		response := request(t, s, "PUT", "/mytopic", `{"n":1}`, map[string]string{
			"X-Message":  `{{printf "%*d" (mul 1000 2000) 1}}`,
			"X-Template": "1",
		})
		require.Equal(t, 400, response.Code)
		require.Contains(t, response.Body.String(), "printf width or precision too large")
	})
}

func TestServer_MessageTemplate_PrintfSmallWidthStillWorks(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		response := request(t, s, "PUT", "/mytopic", `{"n":7}`, map[string]string{
			"X-Message":  `{{printf "%05d" 7}}`,
			"X-Template": "1",
		})
		require.Equal(t, 200, response.Code)
		require.Equal(t, "00007", toMessage(t, response.Body.String()).Message)
	})
}

func Test_templatePrintf(t *testing.T) {
	tests := []struct {
		format string
		args   []any
		want   string // Empty means the call must be rejected
	}{
		{"%d", []any{5}, "5"},
		{"%05d", []any{5}, "00005"},
		{"%-8.3f|", []any{1.5}, "1.500   |"},
		{"%1000d", []any{1}, ""}, // Rejected: four digits
		{"%.1000s", []any{"x"}, ""},
		{"%*d", []any{500, 1}, ""},                           // Rejected: star width
		{"%.*s", []any{400, "x"}, ""},                        // Rejected: star precision
		{"%[1]1000000d", []any{1}, ""},                       // Rejected: explicit arg index does not hide the width
		{"%[2]*[1]d", []any{6, 12}, ""},                      // Rejected: star width behind an arg index
		{"100%% of 2024 values", nil, "100% of 2024 values"}, // Literal digits are not a width
	}
	for _, test := range tests {
		out, err := templatePrintf(test.format, test.args...)
		if test.want == "" {
			require.Error(t, err, "format %q must be rejected", test.format)
			require.Contains(t, err.Error(), "too large")
		} else {
			require.Nil(t, err, "format %q", test.format)
			require.Equal(t, test.want, out)
		}
	}

	// The largest allowed width still produces bounded output
	out, err := templatePrintf("%999d", 1)
	require.Nil(t, err)
	require.Len(t, out, 999)
}

func TestServer_MessageTemplate_DisallowedCallInChain(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		// {{call}} behind a field access parses into a ChainNode. JSON data cannot produce a
		// function value, so this cannot be exploited today, but the ban must catch every
		// syntactic form rather than relying on the call failing at runtime.
		response := request(t, s, "PUT", "/mytopic", `{"fn":1}`, map[string]string{
			"X-Message":  `{{(call .fn).x}}`,
			"X-Template": "1",
		})
		require.Equal(t, 400, response.Code)
		require.Equal(t, 40044, toHTTPError(t, response.Body.String()).Code)
	})
}
