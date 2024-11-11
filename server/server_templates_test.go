package server

import (
	"io"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTemplateFeature(t *testing.T) {
	var testCases = map[string]struct {
		have string
		want templateFeature
	}{
		"empty string": {},
		"invalid string": {
			have: "bogus",
		},
		"boolean 0": {
			have: "0",
		},
		"boolean n": {
			have: "n",
		},
		"boolean y": {
			have: "y",
		},
		"boolean no": {
			have: "no",
		},
		"boolean false": {
			have: "false",
		},
		"boolean 1": {
			have: "1",
			want: templateFeatureInline,
		},
		"boolean yes": {
			have: "yes",
			want: templateFeatureInline,
		},
		"boolean true": {
			have: "true",
			want: templateFeatureInline,
		},
		"explicitly inline": {
			have: "inline",
			want: templateFeatureInline,
		},
		"explicitly server": {
			have: "server",
			want: templateFeatureServer,
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			got := parseTemplateFeature(test.have)
			if got != test.want {
				t.Fatalf("parseTemplateFeature(%q) returned %q; expected %q", test.have, got, test.want)
			}
		})
	}
}

func TestServer_Templates_TemplateFilePath(t *testing.T) {
	s := newTestServer(t, newTestConfigWithTemplates(t))
	var testCases = map[string]struct {
		have string
		want string
	}{
		"empty string": {
			// filepath.Join cleans the output which would interfer with the purpose of this test case
			want: s.config.TemplateDirectory + string(filepath.Separator) + "." + templateExtension,
		},
		"directory traversal": {
			have: "../../../../etc/shadow",
			want: filepath.Join(s.config.TemplateDirectory, "shadow") + templateExtension,
		},
		"relative path": {
			have: "./foo/bar",
			want: filepath.Join(s.config.TemplateDirectory, "bar") + templateExtension,
		},
		"file extension": {
			have: "test.json",
			want: filepath.Join(s.config.TemplateDirectory, "test.json") + templateExtension,
		},
		"simple value": {
			have: "test",
			want: filepath.Join(s.config.TemplateDirectory, "test") + templateExtension,
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			got := s.templateFilePath(test.have)
			if got != test.want {
				t.Fatalf("Server.templateFilePath(%q) returned %q; expected %q", test.have, got, test.want)
			}
		})
	}
}

func TestServer_Templates_ServeTemplate(t *testing.T) {
	s := newTestServer(t, newTestConfigWithTemplates(t))
	var testCases = map[string]struct {
		have      string
		want      string
		wantError bool
	}{
		"empty string": {
			wantError: true,
		},
		"file not found": {
			have:      "404",
			wantError: true,
		},
		"directory index": {
			have:      "index.html",
			wantError: true,
		},
		"valid template foo_message": {
			have: "foo_message",
			want: "{{.foo}}",
		},
		"valid template nested_title": {
			have: "nested_title",
			want: "{{.nested.title}}",
		},
		"valid template foo_repeat": {
			have: "foo_repeat",
			want: "{{.foo}} is {{.foo}}",
		},
		"valid template nested_repeat": {
			have: "nested_repeat",
			want: "{{.nested.title}} is {{.nested.title}}",
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://ntfy.test/foo?tpl=server&m="+test.have, nil)
			w := httptest.NewRecorder()
			err := s.serveTemplate(w, r, test.have)

			if test.wantError {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)

				resp := w.Result()
				body, err := io.ReadAll(resp.Body)
				require.Nil(t, err)
				got := string(body)

				if got != test.want {
					t.Fatalf("Server.serveTemplate(_, _, %q) served %q; expected %q", test.have, got, test.want)
				}
			}
		})
	}
}

func TestServer_Templates_ListTemplates(t *testing.T) {
	s := newTestServer(t, newTestConfigWithTemplates(t))
	got, err := s.listTemplates()

	require.Nil(t, err)

	require.Len(t, got, 4)
	require.Contains(t, got, "foo_message")
	require.Contains(t, got, "nested_title")
	require.Contains(t, got, "foo_repeat")
	require.Contains(t, got, "nested_repeat")

}
