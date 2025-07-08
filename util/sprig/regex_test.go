package sprig

import (
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
