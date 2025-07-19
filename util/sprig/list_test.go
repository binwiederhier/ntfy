package sprig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTuple(t *testing.T) {
	tpl := `{{$t := tuple 1 "a" "foo"}}{{index $t 2}}{{index $t 0 }}{{index $t 1}}`
	if err := runt(tpl, "foo1a"); err != nil {
		t.Error(err)
	}
}

func TestList(t *testing.T) {
	tpl := `{{$t := list 1 "a" "foo"}}{{index $t 2}}{{index $t 0 }}{{index $t 1}}`
	if err := runt(tpl, "foo1a"); err != nil {
		t.Error(err)
	}
}

func TestPush(t *testing.T) {
	// Named `append` in the function map
	tests := map[string]string{
		`{{ $t := tuple 1 2 3  }}{{ append $t 4 | len }}`:                             "4",
		`{{ $t := tuple 1 2 3 4  }}{{ append $t 5 | join "-" }}`:                      "1-2-3-4-5",
		`{{ $t := regexSplit "/" "foo/bar/baz" -1 }}{{ append $t "qux" | join "-" }}`: "foo-bar-baz-qux",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustPush(t *testing.T) {
	// Named `append` in the function map
	tests := map[string]string{
		`{{ $t := tuple 1 2 3  }}{{ mustAppend $t 4 | len }}`:                           "4",
		`{{ $t := tuple 1 2 3 4  }}{{ mustAppend $t 5 | join "-" }}`:                    "1-2-3-4-5",
		`{{ $t := regexSplit "/" "foo/bar/baz" -1 }}{{ mustPush $t "qux" | join "-" }}`: "foo-bar-baz-qux",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestChunk(t *testing.T) {
	tests := map[string]string{
		`{{ tuple 1 2 3 4 5 6 7 | chunk 3 | len }}`:                                 "3",
		`{{ tuple | chunk 3 | len }}`:                                               "0",
		`{{ range ( tuple 1 2 3 4 5 6 7 8 9 | chunk 3 ) }}{{. | join "-"}}|{{end}}`: "1-2-3|4-5-6|7-8-9|",
		`{{ range ( tuple 1 2 3 4 5 6 7 8 | chunk 3 ) }}{{. | join "-"}}|{{end}}`:   "1-2-3|4-5-6|7-8|",
		`{{ range ( tuple 1 2 | chunk 3 ) }}{{. | join "-"}}|{{end}}`:               "1-2|",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustChunk(t *testing.T) {
	tests := map[string]string{
		`{{ tuple 1 2 3 4 5 6 7 | mustChunk 3 | len }}`:                                 "3",
		`{{ tuple | mustChunk 3 | len }}`:                                               "0",
		`{{ range ( tuple 1 2 3 4 5 6 7 8 9 | mustChunk 3 ) }}{{. | join "-"}}|{{end}}`: "1-2-3|4-5-6|7-8-9|",
		`{{ range ( tuple 1 2 3 4 5 6 7 8 | mustChunk 3 ) }}{{. | join "-"}}|{{end}}`:   "1-2-3|4-5-6|7-8|",
		`{{ range ( tuple 1 2 | mustChunk 3 ) }}{{. | join "-"}}|{{end}}`:               "1-2|",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestPrepend(t *testing.T) {
	tests := map[string]string{
		`{{ $t := tuple 1 2 3  }}{{ prepend $t 0 | len }}`:                             "4",
		`{{ $t := tuple 1 2 3 4  }}{{ prepend $t 0 | join "-" }}`:                      "0-1-2-3-4",
		`{{ $t := regexSplit "/" "foo/bar/baz" -1 }}{{ prepend $t "qux" | join "-" }}`: "qux-foo-bar-baz",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustPrepend(t *testing.T) {
	tests := map[string]string{
		`{{ $t := tuple 1 2 3  }}{{ mustPrepend $t 0 | len }}`:                             "4",
		`{{ $t := tuple 1 2 3 4  }}{{ mustPrepend $t 0 | join "-" }}`:                      "0-1-2-3-4",
		`{{ $t := regexSplit "/" "foo/bar/baz" -1 }}{{ mustPrepend $t "qux" | join "-" }}`: "qux-foo-bar-baz",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestFirst(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | first }}`:                          "1",
		`{{ list | first }}`:                                "<no value>",
		`{{ regexSplit "/src/" "foo/src/bar" -1 | first }}`: "foo",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustFirst(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | mustFirst }}`:                          "1",
		`{{ list | mustFirst }}`:                                "<no value>",
		`{{ regexSplit "/src/" "foo/src/bar" -1 | mustFirst }}`: "foo",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestLast(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | last }}`:                          "3",
		`{{ list | last }}`:                                "<no value>",
		`{{ regexSplit "/src/" "foo/src/bar" -1 | last }}`: "bar",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustLast(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | mustLast }}`:                          "3",
		`{{ list | mustLast }}`:                                "<no value>",
		`{{ regexSplit "/src/" "foo/src/bar" -1 | mustLast }}`: "bar",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestInitial(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | initial | len }}`:                "2",
		`{{ list 1 2 3 | initial | last }}`:               "2",
		`{{ list 1 2 3 | initial | first }}`:              "1",
		`{{ list | initial }}`:                            "[]",
		`{{ regexSplit "/" "foo/bar/baz" -1 | initial }}`: "[foo bar]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustInitial(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | mustInitial | len }}`:                "2",
		`{{ list 1 2 3 | mustInitial | last }}`:               "2",
		`{{ list 1 2 3 | mustInitial | first }}`:              "1",
		`{{ list | mustInitial }}`:                            "[]",
		`{{ regexSplit "/" "foo/bar/baz" -1 | mustInitial }}`: "[foo bar]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestRest(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | rest | len }}`:                "2",
		`{{ list 1 2 3 | rest | last }}`:               "3",
		`{{ list 1 2 3 | rest | first }}`:              "2",
		`{{ list | rest }}`:                            "[]",
		`{{ regexSplit "/" "foo/bar/baz" -1 | rest }}`: "[bar baz]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustRest(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | mustRest | len }}`:                "2",
		`{{ list 1 2 3 | mustRest | last }}`:               "3",
		`{{ list 1 2 3 | mustRest | first }}`:              "2",
		`{{ list | mustRest }}`:                            "[]",
		`{{ regexSplit "/" "foo/bar/baz" -1 | mustRest }}`: "[bar baz]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestReverse(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | reverse | first }}`:              "3",
		`{{ list 1 2 3 | reverse | rest | first }}`:       "2",
		`{{ list 1 2 3 | reverse | last }}`:               "1",
		`{{ list 1 2 3 4 | reverse }}`:                    "[4 3 2 1]",
		`{{ list 1 | reverse }}`:                          "[1]",
		`{{ list | reverse }}`:                            "[]",
		`{{ regexSplit "/" "foo/bar/baz" -1 | reverse }}`: "[baz bar foo]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustReverse(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | mustReverse | first }}`:              "3",
		`{{ list 1 2 3 | mustReverse | rest | first }}`:       "2",
		`{{ list 1 2 3 | mustReverse | last }}`:               "1",
		`{{ list 1 2 3 4 | mustReverse }}`:                    "[4 3 2 1]",
		`{{ list 1 | mustReverse }}`:                          "[1]",
		`{{ list | mustReverse }}`:                            "[]",
		`{{ regexSplit "/" "foo/bar/baz" -1 | mustReverse }}`: "[baz bar foo]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestCompact(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 0 "" "hello" | compact }}`:          `[1 hello]`,
		`{{ list "" "" | compact }}`:                   `[]`,
		`{{ list | compact }}`:                         `[]`,
		`{{ regexSplit "/" "foo//bar" -1 | compact }}`: "[foo bar]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustCompact(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 0 "" "hello" | mustCompact }}`:          `[1 hello]`,
		`{{ list "" "" | mustCompact }}`:                   `[]`,
		`{{ list | mustCompact }}`:                         `[]`,
		`{{ regexSplit "/" "foo//bar" -1 | mustCompact }}`: "[foo bar]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestUniq(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 4 | uniq }}`:                    `[1 2 3 4]`,
		`{{ list "a" "b" "c" "d" | uniq }}`:            `[a b c d]`,
		`{{ list 1 1 1 1 2 2 2 2 | uniq }}`:            `[1 2]`,
		`{{ list "foo" 1 1 1 1 "foo" "foo" | uniq }}`:  `[foo 1]`,
		`{{ list | uniq }}`:                            `[]`,
		`{{ regexSplit "/" "foo/foo/bar" -1 | uniq }}`: "[foo bar]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustUniq(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 4 | mustUniq }}`:                    `[1 2 3 4]`,
		`{{ list "a" "b" "c" "d" | mustUniq }}`:            `[a b c d]`,
		`{{ list 1 1 1 1 2 2 2 2 | mustUniq }}`:            `[1 2]`,
		`{{ list "foo" 1 1 1 1 "foo" "foo" | mustUniq }}`:  `[foo 1]`,
		`{{ list | mustUniq }}`:                            `[]`,
		`{{ regexSplit "/" "foo/foo/bar" -1 | mustUniq }}`: "[foo bar]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestWithout(t *testing.T) {
	tests := map[string]string{
		`{{ without (list 1 2 3 4) 1 }}`:                         `[2 3 4]`,
		`{{ without (list "a" "b" "c" "d") "a" }}`:               `[b c d]`,
		`{{ without (list 1 1 1 1 2) 1 }}`:                       `[2]`,
		`{{ without (list) 1 }}`:                                 `[]`,
		`{{ without (list 1 2 3) }}`:                             `[1 2 3]`,
		`{{ without list }}`:                                     `[]`,
		`{{ without (regexSplit "/" "foo/bar/baz" -1 ) "foo" }}`: "[bar baz]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustWithout(t *testing.T) {
	tests := map[string]string{
		`{{ mustWithout (list 1 2 3 4) 1 }}`:                         `[2 3 4]`,
		`{{ mustWithout (list "a" "b" "c" "d") "a" }}`:               `[b c d]`,
		`{{ mustWithout (list 1 1 1 1 2) 1 }}`:                       `[2]`,
		`{{ mustWithout (list) 1 }}`:                                 `[]`,
		`{{ mustWithout (list 1 2 3) }}`:                             `[1 2 3]`,
		`{{ mustWithout list }}`:                                     `[]`,
		`{{ mustWithout (regexSplit "/" "foo/bar/baz" -1 ) "foo" }}`: "[bar baz]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestHas(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | has 1 }}`:                          `true`,
		`{{ list 1 2 3 | has 4 }}`:                          `false`,
		`{{ regexSplit "/" "foo/bar/baz" -1 | has "bar" }}`: `true`,
		`{{ has "bar" nil }}`:                               `false`,
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustHas(t *testing.T) {
	tests := map[string]string{
		`{{ list 1 2 3 | mustHas 1 }}`:                          `true`,
		`{{ list 1 2 3 | mustHas 4 }}`:                          `false`,
		`{{ regexSplit "/" "foo/bar/baz" -1 | mustHas "bar" }}`: `true`,
		`{{ mustHas "bar" nil }}`:                               `false`,
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestSlice(t *testing.T) {
	tests := map[string]string{
		`{{ slice (list 1 2 3) }}`:                          "[1 2 3]",
		`{{ slice (list 1 2 3) 0 1 }}`:                      "[1]",
		`{{ slice (list 1 2 3) 1 3 }}`:                      "[2 3]",
		`{{ slice (list 1 2 3) 1 }}`:                        "[2 3]",
		`{{ slice (regexSplit "/" "foo/bar/baz" -1) 1 2 }}`: "[bar]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestMustSlice(t *testing.T) {
	tests := map[string]string{
		`{{ mustSlice (list 1 2 3) }}`:                          "[1 2 3]",
		`{{ mustSlice (list 1 2 3) 0 1 }}`:                      "[1]",
		`{{ mustSlice (list 1 2 3) 1 3 }}`:                      "[2 3]",
		`{{ mustSlice (list 1 2 3) 1 }}`:                        "[2 3]",
		`{{ mustSlice (regexSplit "/" "foo/bar/baz" -1) 1 2 }}`: "[bar]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}

func TestConcat(t *testing.T) {
	tests := map[string]string{
		`{{ concat (list 1 2 3) }}`:                                   "[1 2 3]",
		`{{ concat (list 1 2 3) (list 4 5) }}`:                        "[1 2 3 4 5]",
		`{{ concat (list 1 2 3) (list 4 5) (list) }}`:                 "[1 2 3 4 5]",
		`{{ concat (list 1 2 3) (list 4 5) (list nil) }}`:             "[1 2 3 4 5 <nil>]",
		`{{ concat (list 1 2 3) (list 4 5) (list ( list "foo" ) ) }}`: "[1 2 3 4 5 [foo]]",
	}
	for tpl, expect := range tests {
		assert.NoError(t, runt(tpl, expect))
	}
}
