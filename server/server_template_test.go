package server

import (
	"context"
	"runtime/debug"
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

func TestServer_MessageTemplate_UntilNegativeStepTooManyIterations(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		// A decreasing sequence must hit the same iteration limit as an increasing one
		response := request(t, s, "PUT", "/mytopic", `{"foo":"bar"}`, map[string]string{
			"X-Message":  `{{len (until -20000)}}`,
			"X-Template": "1",
		})
		require.Equal(t, 400, response.Code)
		require.Equal(t, 40045, toHTTPError(t, response.Body.String()).Code)
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
		require.Contains(t, response.Body.String(), "printf directive not allowed")
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
		require.Contains(t, response.Body.String(), "printf directive not allowed")
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
		require.Contains(t, response.Body.String(), "printf directive not allowed")
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

func TestServer_MessageTemplate_ReplaceAmplification(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		// replace with an empty old string is len^2 inside one function call: 8 KB -> 64 MB
		response := request(t, s, "PUT", "/mytopic", `{"a":"`+strings.Repeat("A", 8*1024)+`"}`, map[string]string{
			"X-Message":  `{{ len (replace "" .a .a) }}`,
			"X-Template": "1",
		})
		require.Equal(t, 400, response.Code)
		require.Equal(t, 40045, toHTTPError(t, response.Body.String()).Code)
		require.Contains(t, response.Body.String(), "exceeds limit")
	})
}

func TestServer_MessageTemplate_SelfReferencingValue(t *testing.T) {
	// Printing a cyclic value recurses forever; a small stack makes the failure fast without the fix
	defer debug.SetMaxStack(debug.SetMaxStack(32 << 20))
	cycle := `{{$a := dict}}{{$b := list $a}}{{$_ := set $a "b" $b}}` // $a -> $b -> $a
	for _, tpl := range []string{
		cycle + `{{toJSON $a | len}}`,
		cycle + `{{len (mustToJSON $a)}}`,
		cycle + `{{len (print $a)}}`,
		cycle + `{{len (toString $a)}}`,
		cycle + `{{$a}}`,
		cycle + `{{if true}}{{$a}}{{end}}`,
		cycle + `{{range list 1}}{{$a}}{{end}}`,
		cycle + `{{with 0}}x{{else}}{{$a}}{{end}}`,
		cycle + `{{$a | toString | len}}{{$a}}`,
	} {
		forEachBackend(t, func(t *testing.T, databaseURL string) {
			s := newTestServer(t, newTestConfig(t, databaseURL))
			response := request(t, s, "PUT", "/mytopic", `{}`, map[string]string{
				"X-Message":  tpl,
				"X-Template": "1",
			})
			require.Equal(t, 400, response.Code, tpl)
			require.Equal(t, 40045, toHTTPError(t, response.Body.String()).Code, tpl)
			require.Contains(t, response.Body.String(), "value references itself", tpl)
		})
	}
}

func TestServer_MessageTemplate_SharedValueTooLarge(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		// Each level references the previous one twice, so 20 levels expand to 2^20 leaves when
		// printed; only the length is written, so the output limit never sees the expansion
		doubling := `{{$a := list 1 1}}` + strings.Repeat(`{{$a = list $a $a}}`, 19)
		for _, tpl := range []string{
			doubling + `{{toJSON $a | len}}`,
			doubling + `{{len (print $a)}}`,
			doubling + `{{len (join "," $a)}}`,
			doubling + `{{$a}}`, // Printed directly by the executor, not by a function
		} {
			response := request(t, s, "PUT", "/mytopic", `{}`, map[string]string{
				"X-Message":  tpl,
				"X-Template": "1",
			})
			require.Equal(t, 400, response.Code, tpl)
			require.Equal(t, 40045, toHTTPError(t, response.Body.String()).Code, tpl)
			require.Contains(t, response.Body.String(), "value is too large", tpl)
		}
	})
}

func TestServer_MessageTemplate_PrintSharedValueStillWorks(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		response := request(t, s, "PUT", "/mytopic", `{"a":{"b":"c"}}`, map[string]string{
			"X-Message":  `{{$l := list .a .a}}{{toJSON $l}} {{$l}}`,
			"X-Template": "1",
		})
		require.Equal(t, 200, response.Code)
		require.Equal(t, `[{"b":"c"},{"b":"c"}] [map[b:c] map[b:c]]`, toMessage(t, response.Body.String()).Message)
	})
}

func TestServer_renderTemplate_StringBuildingLimit(t *testing.T) {
	// The variadic builtins format every argument into one string inside a single call, where the
	// output limit and the deadline never look. $b is 90 KB; three references are past the
	// 256 KiB print budget.
	s := newTestServer(t, newTestConfig(t, ""))
	const b = `{{$b := repeat 9000 "aaaaaaaaaa"}}`
	refs := strings.Repeat(" $b", 3)
	rejected := map[string]string{
		b + `{{len (print` + refs + `)}}`:        "value is too large",
		b + `{{len (println` + refs + `)}}`:      "value is too large",
		b + `{{len (print (list` + refs + `))}}`: "value is too large",
		b + `{{len (printf "%[1]s%[1]s" $b)}}`:   "argument index",
		b + `{{len (printf "%v" (list $b $b))}}`: "lists or dicts",
		`{{len (printf "%999d" (until 200))}}`:   "lists or dicts",
	}
	for tpl, want := range rejected {
		_, err := s.renderTemplate(context.Background(), "test", tpl, "{}")
		require.Error(t, err, "template %s must be rejected", tpl)
		require.Contains(t, err.Error(), want, "template %s", tpl)
	}
	allowed := map[string]string{
		b + `{{len (print $b $b)}}`:                   "180000",
		b + `{{len (println $b $b)}}`:                 "180002",
		`{{printf "%s=%05d %.2f %v" "n" 7 1.5 true}}`: "n=00007 1.50 true",
		`{{print "a" 1 2 "b"}}|{{println "c"}}`:       "a1 2b|c",
	}
	for tpl, want := range allowed {
		out, err := s.renderTemplate(context.Background(), "test", tpl, "{}")
		require.Nil(t, err, "template %s", tpl)
		require.Equal(t, want, out, "template %s", tpl)
	}
}

func TestServer_MessageTemplate_EscapersDisallowed(t *testing.T) {
	// The html, js and urlquery escapers are of no use in a notification, so they are rejected
	// when parsing
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		t.Parallel()
		s := newTestServer(t, newTestConfig(t, databaseURL))
		for _, tpl := range []string{`{{html "<a>"}}`, `{{js "a"}}`, `{{"<a>" | html}}`, `{{if js "a"}}x{{end}}`, `{{urlquery "a b"}}`} {
			response := request(t, s, "PUT", "/mytopic", `{}`, map[string]string{
				"X-Message":  tpl,
				"X-Template": "1",
			})
			require.Equal(t, 400, response.Code, tpl)
			require.Equal(t, 40044, toHTTPError(t, response.Body.String()).Code, tpl)
		}
	})
}

func TestServer_renderTemplate_OutputActionsUnchanged(t *testing.T) {
	// Every output action is piped through a guarded print function; it must render exactly like
	// the builtin printing, including "<no value>" for missing keys and JSON nulls
	s := newTestServer(t, newTestConfig(t, ""))
	source := `{"a":1,"f":2.5,"n":null,"s":"x","b":true,"l":[1,"b"],"m":{"k":"v"}}`
	tests := map[string]string{
		`{{.a}}|{{.f}}|{{.n}}|{{.s}}|{{.b}}`:                           "1|2.5|<no value>|x|true",
		`{{.missing}}|{{.missing.deeper}}`:                             "<no value>|<no value>",
		`{{.l}}|{{.m}}|{{.l | len}}`:                                   "[1 b]|map[k:v]|2",
		`{{$x := 5}}{{$x}}{{$x = 6}}{{$x}}`:                            "56",
		`{{if .a}}{{.s}}{{else}}no{{end}}`:                             "x",
		`{{range .l}}{{.}},{{end}}{{range .nope}}x{{else}}e{{end}}`:    "1,b,e",
		`{{with .m}}{{.k}}{{end}}{{with .nope}}x{{else}}{{.s}}{{end}}`: "vx",
		`{{range $i, $v := .l}}{{$i}}={{$v}} {{end}}`:                  "0=1 1=b",
		`{{printf "%s-%d" .s 3}}{{print .s}}{{"lit"}}{{7}}`:            "x-3xlit7",
	}
	for tpl, want := range tests {
		out, err := s.renderTemplate(context.Background(), "test", tpl, source)
		require.Nil(t, err, "template %s", tpl)
		require.Equal(t, want, out, "template %s", tpl)
	}
}

func TestServer_MessageTemplate_ExecuteErrorNamesParameter(t *testing.T) {
	// An error in an inline template must name the parameter that contains the broken template
	s := newTestServer(t, newTestConfig(t, ""))
	tests := map[string]string{
		"X-Message":  "template message query parameter: ",
		"X-Title":    "template title query parameter: ",
		"X-Priority": "template priority query parameter: ",
	}
	for header, want := range tests {
		response := request(t, s, "PUT", "/mytopic", `{"foo":"bar"}`, map[string]string{
			header:       `{{fail "boom"}}`,
			"X-Template": "1",
		})
		require.Equal(t, 400, response.Code, header)
		err := toHTTPError(t, response.Body.String())
		require.Equal(t, errHTTPBadRequestTemplateExecuteFailed.Code, err.Code, header)
		require.Contains(t, err.Message, want, header)
	}
}
