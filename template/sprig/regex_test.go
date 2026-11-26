package sprig

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegexMatch(t *testing.T) {
	regex := "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}"

	assert.True(t, regexMatch(regex, "test@acme.com"))
	assert.True(t, regexMatch(regex, "Test@Acme.Com"))
	assert.False(t, regexMatch(regex, "test"))
	assert.False(t, regexMatch(regex, "test.com"))
	assert.False(t, regexMatch(regex, "test@acme"))
}

func TestMustRegexMatch(t *testing.T) {
	regex := "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}"

	o, err := mustRegexMatch(regex, "test@acme.com")
	assert.True(t, o)
	assert.Nil(t, err)

	o, err = mustRegexMatch(regex, "Test@Acme.Com")
	assert.True(t, o)
	assert.Nil(t, err)

	o, err = mustRegexMatch(regex, "test")
	assert.False(t, o)
	assert.Nil(t, err)

	o, err = mustRegexMatch(regex, "test.com")
	assert.False(t, o)
	assert.Nil(t, err)

	o, err = mustRegexMatch(regex, "test@acme")
	assert.False(t, o)
	assert.Nil(t, err)
}

func TestRegexFindAll(t *testing.T) {
	regex := "a{2}"
	assert.Equal(t, 1, len(regexFindAll(regex, "aa", -1)))
	assert.Equal(t, 1, len(regexFindAll(regex, "aaaaaaaa", 1)))
	assert.Equal(t, 2, len(regexFindAll(regex, "aaaa", -1)))
	assert.Equal(t, 0, len(regexFindAll(regex, "none", -1)))
}

func TestMustRegexFindAll(t *testing.T) {
	type args struct {
		regex, s string
		n        int
	}
	cases := []struct {
		expected int
		args     args
	}{
		{1, args{"a{2}", "aa", -1}},
		{1, args{"a{2}", "aaaaaaaa", 1}},
		{2, args{"a{2}", "aaaa", -1}},
		{0, args{"a{2}", "none", -1}},
	}

	for _, c := range cases {
		res, err := mustRegexFindAll(c.args.regex, c.args.s, c.args.n)
		if err != nil {
			t.Errorf("regexFindAll test case %v failed with err %s", c, err)
		}
		assert.Equal(t, c.expected, len(res), "case %#v", c.args)
	}
}

func TestRegexFindl(t *testing.T) {
	regex := "fo.?"
	assert.Equal(t, "foo", regexFind(regex, "foorbar"))
	assert.Equal(t, "foo", regexFind(regex, "foo foe fome"))
	assert.Equal(t, "", regexFind(regex, "none"))
}

func TestMustRegexFindl(t *testing.T) {
	type args struct{ regex, s string }
	cases := []struct {
		expected string
		args     args
	}{
		{"foo", args{"fo.?", "foorbar"}},
		{"foo", args{"fo.?", "foo foe fome"}},
		{"", args{"fo.?", "none"}},
	}

	for _, c := range cases {
		res, err := mustRegexFind(c.args.regex, c.args.s)
		if err != nil {
			t.Errorf("regexFind test case %v failed with err %s", c, err)
		}
		assert.Equal(t, c.expected, res, "case %#v", c.args)
	}
}

func TestRegexReplaceAll(t *testing.T) {
	regex := "a(x*)b"
	assert.Equal(t, "-T-T-", regexReplaceAll(regex, "-ab-axxb-", "T"))
	assert.Equal(t, "--xx-", regexReplaceAll(regex, "-ab-axxb-", "$1"))
	assert.Equal(t, "---", regexReplaceAll(regex, "-ab-axxb-", "$1W"))
	assert.Equal(t, "-W-xxW-", regexReplaceAll(regex, "-ab-axxb-", "${1}W"))
}

func TestRegexReplaceAllLimit(t *testing.T) {
	// An empty-ish pattern matches at every position, so regexReplaceAll can emit
	// (len(s)+1) * len(repl) bytes from small inputs (same DoS class as replace).
	big := strings.Repeat("a", 2000) // (len+1)*len ~ 4 MB result, past the 100k limit
	for _, f := range []func() string{
		func() string { return regexReplaceAll("x?", big, big) },
		func() string { return regexReplaceAllLiteral("x?", big, big) },
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Error("expected the oversized regex replace to panic")
				} else if !strings.Contains(fmt.Sprint(r), "exceeds limit") {
					t.Errorf("expected a limit panic, got: %v", r)
				}
			}()
			f()
		}()
	}
}

func TestMustRegexReplaceAll(t *testing.T) {
	type args struct{ regex, s, repl string }
	cases := []struct {
		expected string
		args     args
	}{
		{"-T-T-", args{"a(x*)b", "-ab-axxb-", "T"}},
		{"--xx-", args{"a(x*)b", "-ab-axxb-", "$1"}},
		{"---", args{"a(x*)b", "-ab-axxb-", "$1W"}},
		{"-W-xxW-", args{"a(x*)b", "-ab-axxb-", "${1}W"}},
	}

	for _, c := range cases {
		res, err := mustRegexReplaceAll(c.args.regex, c.args.s, c.args.repl)
		if err != nil {
			t.Errorf("regexReplaceAll test case %v failed with err %s", c, err)
		}
		assert.Equal(t, c.expected, res, "case %#v", c.args)
	}
}

func TestRegexReplaceAllLiteral(t *testing.T) {
	regex := "a(x*)b"
	assert.Equal(t, "-T-T-", regexReplaceAllLiteral(regex, "-ab-axxb-", "T"))
	assert.Equal(t, "-$1-$1-", regexReplaceAllLiteral(regex, "-ab-axxb-", "$1"))
	assert.Equal(t, "-${1}-${1}-", regexReplaceAllLiteral(regex, "-ab-axxb-", "${1}"))
}

func TestMustRegexReplaceAllLiteral(t *testing.T) {
	type args struct{ regex, s, repl string }
	cases := []struct {
		expected string
		args     args
	}{
		{"-T-T-", args{"a(x*)b", "-ab-axxb-", "T"}},
		{"-$1-$1-", args{"a(x*)b", "-ab-axxb-", "$1"}},
		{"-${1}-${1}-", args{"a(x*)b", "-ab-axxb-", "${1}"}},
	}

	for _, c := range cases {
		res, err := mustRegexReplaceAllLiteral(c.args.regex, c.args.s, c.args.repl)
		if err != nil {
			t.Errorf("regexReplaceAllLiteral test case %v failed with err %s", c, err)
		}
		assert.Equal(t, c.expected, res, "case %#v", c.args)
	}
}

func TestRegexSplit(t *testing.T) {
	regex := "a"
	assert.Equal(t, 4, len(regexSplit(regex, "banana", -1)))
	assert.Equal(t, 0, len(regexSplit(regex, "banana", 0)))
	assert.Equal(t, 1, len(regexSplit(regex, "banana", 1)))
	assert.Equal(t, 2, len(regexSplit(regex, "banana", 2)))

	regex = "z+"
	assert.Equal(t, 2, len(regexSplit(regex, "pizza", -1)))
	assert.Equal(t, 0, len(regexSplit(regex, "pizza", 0)))
	assert.Equal(t, 1, len(regexSplit(regex, "pizza", 1)))
	assert.Equal(t, 2, len(regexSplit(regex, "pizza", 2)))
}

func TestMustRegexSplit(t *testing.T) {
	type args struct {
		regex, s string
		n        int
	}
	cases := []struct {
		expected int
		args     args
	}{
		{4, args{"a", "banana", -1}},
		{0, args{"a", "banana", 0}},
		{1, args{"a", "banana", 1}},
		{2, args{"a", "banana", 2}},
		{2, args{"z+", "pizza", -1}},
		{0, args{"z+", "pizza", 0}},
		{1, args{"z+", "pizza", 1}},
		{2, args{"z+", "pizza", 2}},
	}

	for _, c := range cases {
		res, err := mustRegexSplit(c.args.regex, c.args.s, c.args.n)
		if err != nil {
			t.Errorf("regexSplit test case %v failed with err %s", c, err)
		}
		assert.Equal(t, c.expected, len(res), "case %#v", c.args)
	}
}

func TestRegexQuoteMeta(t *testing.T) {
	assert.Equal(t, "1\\.2\\.3", regexQuoteMeta("1.2.3"))
	assert.Equal(t, "pretzel", regexQuoteMeta("pretzel"))
}

func TestRegex_RejectsLargeProgram(t *testing.T) {
	// Go rejects nested repeats like a{1000}{10} itself, but accepts concatenated ones; each
	// "a{1000}" compiles to ~1000 instructions, so six of them exceed maxRegexProgramInsts
	regex := strings.Repeat("a{1000}", 6)

	assert.False(t, regexMatch(regex, strings.Repeat("a", 6000)))
	_, err := mustRegexMatch(regex, "aaa")
	assert.Error(t, err)

	assert.Panics(t, func() { regexFindAll(regex, "aaa", -1) })
	_, err = mustRegexFindAll(regex, "aaa", -1)
	assert.Error(t, err)

	assert.Panics(t, func() { regexFind(regex, "aaa") })
	_, err = mustRegexFind(regex, "aaa")
	assert.Error(t, err)

	assert.Panics(t, func() { regexReplaceAll(regex, "aaa", "b") })
	_, err = mustRegexReplaceAll(regex, "aaa", "b")
	assert.Error(t, err)

	assert.Panics(t, func() { regexReplaceAllLiteral(regex, "aaa", "b") })
	_, err = mustRegexReplaceAllLiteral(regex, "aaa", "b")
	assert.Error(t, err)

	assert.Panics(t, func() { regexSplit(regex, "aaa", -1) })
	_, err = mustRegexSplit(regex, "aaa", -1)
	assert.Error(t, err)
}

func TestRegex_RejectsLongPattern(t *testing.T) {
	regex := strings.Repeat("a", maxRegexPatternLength+1)
	_, err := mustRegexMatch(regex, "aaa")
	assert.Error(t, err)
	assert.Panics(t, func() { regexFind(regex, "aaa") })
}

func TestRegex_AllowsNormalPatterns(t *testing.T) {
	// Patterns people actually use in notification templates stay well below the limits; the
	// largest here, an email pattern with RFC length bounds, compiles to about 760 instructions
	tests := []struct {
		regex string
		s     string
		want  string
	}{
		{`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, "phil@example.com", "phil@example.com"},
		{`\b[A-Za-z0-9._%+-]{1,64}@[A-Za-z0-9.-]{1,253}\.[A-Za-z]{2,63}\b`, "mail ops@example.org now", "ops@example.org"},
		{`\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b`, "host 192.168.1.254 down", "192.168.1.254"},
		{`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`, "id=123e4567-e89b-12d3-a456-426614174000", "123e4567-e89b-12d3-a456-426614174000"},
		{`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`, "at 2026-11-16T09:30:00.123Z", "2026-11-16T09:30:00.123Z"},
		{`https?://[^\s/$.?#].[^\s]*`, "see https://ntfy.sh/docs for more", "https://ntfy.sh/docs"},
		{`v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?`, "released v2.38.1-rc.1", "v2.38.1-rc.1"},
		{`(?i)\b(error|fatal|panic|critical)\b`, "Disk CRITICAL on app-3", "CRITICAL"},
		{`[\p{L}\p{N}]+`, "  Grüße ", "Grüße"},
		{`\d{1,3}(,\d{3})*(\.\d{2})?`, "total $1,234,567.89", "1,234,567.89"},
		{`a{1,100}`, "aaa", "aaa"},
	}
	for _, test := range tests {
		got, err := mustRegexFind(test.regex, test.s)
		assert.Nil(t, err, test.regex)
		assert.Equal(t, test.want, got, test.regex)
		assert.Equal(t, test.want, regexFind(test.regex, test.s), test.regex)
	}

	// The other functions, as used in a template
	for tpl, want := range map[string]string{
		`{{ regexMatch "^[A-Z]+-[0-9]+$" "OPS-1234" }}`:                                "true",
		`{{ regexFindAll "[0-9]+" "cpu 93, mem 71, disk 88" -1 | join "," }}`:          "93,71,88",
		`{{ regexReplaceAll "(?P<user>[a-z]+)@(?P<host>[a-z.]+)" "a@b.c" "${host}" }}`: "b.c",
		`{{ regexReplaceAllLiteral "\\s+" "too   many    spaces" " " }}`:               "too many spaces",
		`{{ regexSplit "\\s*,\\s*" "a, b ,c" -1 | join "|" }}`:                         "a|b|c",
		`{{ mustRegexMatch "(?m)^ERROR" "ok\nERROR: boom" }}`:                          "true",
	} {
		assert.Nil(t, runt(tpl, want), tpl)
	}

	// A pattern of exactly the maximum length is fine
	_, err := mustRegexMatch(strings.Repeat("a", maxRegexPatternLength), "aaa")
	assert.Nil(t, err)
}
