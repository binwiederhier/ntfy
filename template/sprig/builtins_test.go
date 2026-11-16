package sprig

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// largeString is a 90 KB string; three references to it exceed the 256 KiB print budget
const largeString = `{{$b := repeat 9000 "aaaaaaaaaa"}}`

func TestTemplatePrintf(t *testing.T) {
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
			require.Contains(t, err.Error(), "printf directive not allowed")
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

func TestTemplatePrintf_ArgumentIndexRejected(t *testing.T) {
	// An explicit argument index prints one argument any number of times
	for _, format := range []string{"%[1]s%[1]s", "%-[1]s", "%[1]d"} {
		_, err := templatePrintf(format, "x")
		require.ErrorContains(t, err, "printf directive not allowed", format)
	}
}

func TestTemplatePrintf_OnlyKnownDirectivesAllowed(t *testing.T) {
	// Anything that is not a plain directive is rejected, including verbs fmt would print as an
	// error, and %p, which prints a memory address
	for _, format := range []string{"%p", "%z", "%!", "trailing %", "%-+# 05.3y"} {
		_, err := templatePrintf(format, 1)
		require.ErrorContains(t, err, "printf directive not allowed", format)
	}
	out, err := templatePrintf("%x %X %o %q %c %t %e %g %T %U %5.1f|%-3s|%+d|%#x|% d", 255, 255, 8, "a", 'z', true, 1.5, 2.5, 1, 0x41, 1.25, "a", 3, 255, 4)
	require.Nil(t, err)
	require.Equal(t, `ff FF 10 "a" z true 1.500000e+00 2.5 int U+0041   1.2|a  |+3|0xff| 4`, out)
}

func TestTemplatePrintf_ListArgumentRejected(t *testing.T) {
	// A width pads every element of a list, so lists and dicts are not printf arguments
	for _, arg := range []any{[]any{1, 2}, []int{1}, map[string]any{"a": 1}} {
		_, err := templatePrintf("%999v", arg)
		require.ErrorContains(t, err, "lists or dicts", "%#v", arg)
	}
	out, err := templatePrintf("%s=%05d %.2f %v %v", "n", 7, 1.5, true, nil)
	require.Nil(t, err)
	require.Equal(t, "n=00007 1.50 true <nil>", out)
}

func TestTemplatePrintf_SizeLimit(t *testing.T) {
	_, err := runRaw(largeString+`{{len (printf "%s%s%s" $b $b $b)}}`, nil)
	require.ErrorContains(t, err, "value is too large")
}

func TestTemplatePrint_SizeLimit(t *testing.T) {
	refs := strings.Repeat(" $b", 3) // 270 KB, past the 256 KiB budget
	for _, tpl := range []string{
		largeString + `{{len (print` + refs + `)}}`,
		largeString + `{{len (println` + refs + `)}}`,
		largeString + `{{len (print (list` + refs + `))}}`, // A list prints all of its elements
	} {
		_, err := runRaw(tpl, nil)
		require.ErrorContains(t, err, "value is too large", tpl)
	}
	out, err := runRaw(largeString+`{{len (print $b $b)}} {{len (println $b $b)}} {{print "a" 1 2 "b"}}`, nil)
	require.Nil(t, err)
	require.Equal(t, "180000 180002 a1 2b", out)
}
