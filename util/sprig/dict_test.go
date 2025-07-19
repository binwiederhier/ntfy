package sprig

import (
	"strings"
	"testing"
)

func TestDict(t *testing.T) {
	tpl := `{{$d := dict 1 2 "three" "four" 5}}{{range $k, $v := $d}}{{$k}}{{$v}}{{end}}`
	out, err := runRaw(tpl, nil)
	if err != nil {
		t.Error(err)
	}
	if len(out) != 12 {
		t.Errorf("Expected length 12, got %d", len(out))
	}
	// dict does not guarantee ordering because it is backed by a map.
	if !strings.Contains(out, "12") {
		t.Error("Expected grouping 12")
	}
	if !strings.Contains(out, "threefour") {
		t.Error("Expected grouping threefour")
	}
	if !strings.Contains(out, "5") {
		t.Error("Expected 5")
	}
	tpl = `{{$t := dict "I" "shot" "the" "albatross"}}{{$t.the}} {{$t.I}}`
	if err := runt(tpl, "albatross shot"); err != nil {
		t.Error(err)
	}
}

func TestUnset(t *testing.T) {
	tpl := `{{- $d := dict "one" 1 "two" 222222 -}}
	{{- $_ := unset $d "two" -}}
	{{- range $k, $v := $d}}{{$k}}{{$v}}{{- end -}}
	`

	expect := "one1"
	if err := runt(tpl, expect); err != nil {
		t.Error(err)
	}
}
func TestHasKey(t *testing.T) {
	tpl := `{{- $d := dict "one" 1 "two" 222222 -}}
	{{- if hasKey $d "one" -}}1{{- end -}}
	`

	expect := "1"
	if err := runt(tpl, expect); err != nil {
		t.Error(err)
	}
}

func TestPluck(t *testing.T) {
	tpl := `
	{{- $d := dict "one" 1 "two" 222222 -}}
	{{- $d2 := dict "one" 1 "two" 33333 -}}
	{{- $d3 := dict "one" 1 -}}
	{{- $d4 := dict "one" 1 "two" 4444 -}}
	{{- pluck "two" $d $d2 $d3 $d4 -}}
	`

	expect := "[222222 33333 4444]"
	if err := runt(tpl, expect); err != nil {
		t.Error(err)
	}
}

func TestKeys(t *testing.T) {
	tests := map[string]string{
		`{{ dict "foo" 1 "bar" 2 | keys | sortAlpha }}`: "[bar foo]",
		`{{ dict | keys }}`:                             "[]",
		`{{ keys (dict "foo" 1) (dict "bar" 2) (dict "bar" 3) | uniq | sortAlpha }}`: "[bar foo]",
	}
	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}
}

func TestPick(t *testing.T) {
	tests := map[string]string{
		`{{- $d := dict "one" 1 "two" 222222 }}{{ pick $d "two" | len -}}`:               "1",
		`{{- $d := dict "one" 1 "two" 222222 }}{{ pick $d "two" -}}`:                     "map[two:222222]",
		`{{- $d := dict "one" 1 "two" 222222 }}{{ pick $d "one" "two" | len -}}`:         "2",
		`{{- $d := dict "one" 1 "two" 222222 }}{{ pick $d "one" "two" "three" | len -}}`: "2",
		`{{- $d := dict }}{{ pick $d "two" | len -}}`:                                    "0",
	}
	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}
}
func TestOmit(t *testing.T) {
	tests := map[string]string{
		`{{- $d := dict "one" 1 "two" 222222 }}{{ omit $d "one" | len -}}`:         "1",
		`{{- $d := dict "one" 1 "two" 222222 }}{{ omit $d "one" -}}`:               "map[two:222222]",
		`{{- $d := dict "one" 1 "two" 222222 }}{{ omit $d "one" "two" | len -}}`:   "0",
		`{{- $d := dict "one" 1 "two" 222222 }}{{ omit $d "two" "three" | len -}}`: "1",
		`{{- $d := dict }}{{ omit $d "two" | len -}}`:                              "0",
	}
	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}
}

func TestGet(t *testing.T) {
	tests := map[string]string{
		`{{- $d := dict "one" 1 }}{{ get $d "one" -}}`:           "1",
		`{{- $d := dict "one" 1 "two" "2" }}{{ get $d "two" -}}`: "2",
		`{{- $d := dict }}{{ get $d "two" -}}`:                   "",
	}
	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}
}

func TestSet(t *testing.T) {
	tpl := `{{- $d := dict "one" 1 "two" 222222 -}}
	{{- $_ := set $d "two" 2 -}}
	{{- $_ := set $d "three" 3 -}}
	{{- if hasKey $d "one" -}}{{$d.one}}{{- end -}}
	{{- if hasKey $d "two" -}}{{$d.two}}{{- end -}}
	{{- if hasKey $d "three" -}}{{$d.three}}{{- end -}}
	`

	expect := "123"
	if err := runt(tpl, expect); err != nil {
		t.Error(err)
	}
}

func TestValues(t *testing.T) {
	tests := map[string]string{
		`{{- $d := dict "a" 1 "b" 2 }}{{ values $d | sortAlpha | join "," }}`:       "1,2",
		`{{- $d := dict "a" "first" "b" 2 }}{{ values $d | sortAlpha | join "," }}`: "2,first",
	}

	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}
}

func TestDig(t *testing.T) {
	tests := map[string]string{
		`{{- $d := dict "a" (dict "b" (dict "c" 1)) }}{{ dig "a" "b" "c" "" $d }}`:  "1",
		`{{- $d := dict "a" (dict "b" (dict "c" 1)) }}{{ dig "a" "b" "z" "2" $d }}`: "2",
		`{{ dict "a" 1 | dig "a" "" }}`:                                             "1",
		`{{ dict "a" 1 | dig "z" "2" }}`:                                            "2",
	}

	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}
}
