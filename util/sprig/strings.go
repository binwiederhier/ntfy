package sprig

import (
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"reflect"
	"strconv"
	"strings"
)

// base64encode encodes a string to base64 using standard encoding.
//
// Parameters:
//   - v: The string to encode
//
// Returns:
//   - string: The base64 encoded string
func base64encode(v string) string {
	return base64.StdEncoding.EncodeToString([]byte(v))
}

// base64decode decodes a base64 encoded string.
// If the input is not valid base64, it returns the error message as a string.
//
// Parameters:
//   - v: The base64 encoded string to decode
//
// Returns:
//   - string: The decoded string, or an error message if decoding fails
func base64decode(v string) string {
	data, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

// base32encode encodes a string to base32 using standard encoding.
//
// Parameters:
//   - v: The string to encode
//
// Returns:
//   - string: The base32 encoded string
func base32encode(v string) string {
	return base32.StdEncoding.EncodeToString([]byte(v))
}

// base32decode decodes a base32 encoded string.
// If the input is not valid base32, it returns the error message as a string.
//
// Parameters:
//   - v: The base32 encoded string to decode
//
// Returns:
//   - string: The decoded string, or an error message if decoding fails
func base32decode(v string) string {
	data, err := base32.StdEncoding.DecodeString(v)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

// quote adds double quotes around each non-nil string in the input and joins them with spaces.
// This uses Go's %q formatter which handles escaping special characters.
//
// Parameters:
//   - str: A variadic list of values to quote
//
// Returns:
//   - string: The quoted strings joined with spaces
func quote(str ...any) string {
	out := make([]string, 0, len(str))
	for _, s := range str {
		if s != nil {
			out = append(out, fmt.Sprintf("%q", strval(s)))
		}
	}
	return strings.Join(out, " ")
}

// squote adds single quotes around each non-nil value in the input and joins them with spaces.
// Unlike quote, this doesn't escape special characters.
//
// Parameters:
//   - str: A variadic list of values to quote
//
// Returns:
//   - string: The single-quoted values joined with spaces
func squote(str ...any) string {
	out := make([]string, 0, len(str))
	for _, s := range str {
		if s != nil {
			out = append(out, fmt.Sprintf("'%v'", s))
		}
	}
	return strings.Join(out, " ")
}

// cat concatenates all non-nil values into a single string.
// Nil values are removed before concatenation.
//
// Parameters:
//   - v: A variadic list of values to concatenate
//
// Returns:
//   - string: The concatenated string
func cat(v ...any) string {
	v = removeNilElements(v)
	r := strings.TrimSpace(strings.Repeat("%v ", len(v)))
	return fmt.Sprintf(r, v...)
}

// indent adds a specified number of spaces at the beginning of each line in a string.
// It has a safety limit to prevent excessive memory usage.
//
// Parameters:
//   - spaces: The number of spaces to add
//   - v: The string to indent
//
// Returns:
//   - string: The indented string
//
// Panics:
//   - If spaces exceeds indentSpacesLimit
func indent(spaces int, v string) string {
	if spaces > indentSpacesLimit {
		panic(fmt.Sprintf("indent %d exceeds limit of %d", spaces, indentSpacesLimit))
	}
	pad := strings.Repeat(" ", spaces)
	return pad + strings.Replace(v, "\n", "\n"+pad, -1)
}

// nindent adds a newline followed by an indented string.
// It's a shorthand for "\n" + indent(spaces, v).
//
// Parameters:
//   - spaces: The number of spaces to add
//   - v: The string to indent
//
// Returns:
//   - string: A newline followed by the indented string
func nindent(spaces int, v string) string {
	return "\n" + indent(spaces, v)
}

// replace replaces all occurrences of a substring with another substring.
//
// Parameters:
//   - old: The substring to replace
//   - new: The replacement substring
//   - src: The source string
//
// Returns:
//   - string: The resulting string after all replacements
func replace(old, new, src string) string {
	return strings.Replace(src, old, new, -1)
}

// plural returns the singular or plural form of a word based on the count.
// If count is 1, it returns the singular form, otherwise it returns the plural form.
//
// Parameters:
//   - one: The singular form of the word
//   - many: The plural form of the word
//   - count: The count to determine which form to use
//
// Returns:
//   - string: Either the singular or plural form based on the count
func plural(one, many string, count int) string {
	if count == 1 {
		return one
	}
	return many
}

// strslice converts a value to a slice of strings.
// It handles various input types:
// - []string: returned as is
// - []any: converted to []string, skipping nil values
// - arrays and slices: converted to []string, skipping nil values
// - nil: returns an empty slice
// - anything else: returns a single-element slice with the string representation
//
// Parameters:
//   - v: The value to convert to a string slice
//
// Returns:
//   - []string: A slice of strings
func strslice(v any) []string {
	switch v := v.(type) {
	case []string:
		return v
	case []any:
		b := make([]string, 0, len(v))
		for _, s := range v {
			if s != nil {
				b = append(b, strval(s))
			}
		}
		return b
	default:
		val := reflect.ValueOf(v)
		switch val.Kind() {
		case reflect.Array, reflect.Slice:
			l := val.Len()
			b := make([]string, 0, l)
			for i := 0; i < l; i++ {
				value := val.Index(i).Interface()
				if value != nil {
					b = append(b, strval(value))
				}
			}
			return b
		default:
			if v == nil {
				return []string{}
			}

			return []string{strval(v)}
		}
	}
}

// removeNilElements creates a new slice with all nil elements removed.
// This is a helper function used by other functions like cat.
//
// Parameters:
//   - v: The slice to process
//
// Returns:
//   - []any: A new slice with all nil elements removed
func removeNilElements(v []any) []any {
	newSlice := make([]any, 0, len(v))
	for _, i := range v {
		if i != nil {
			newSlice = append(newSlice, i)
		}
	}
	return newSlice
}

// strval converts any value to a string.
// It handles various types:
// - string: returned as is
// - []byte: converted to string
// - error: returns the error message
// - fmt.Stringer: calls the String() method
// - anything else: uses fmt.Sprintf("%v", v)
//
// Parameters:
//   - v: The value to convert to a string
//
// Returns:
//   - string: The string representation of the value
func strval(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case error:
		return v.Error()
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// trunc truncates a string to a specified length.
// If c is positive, it returns the first c characters.
// If c is negative, it returns the last |c| characters.
// If the string is shorter than the requested length, it returns the original string.
//
// Parameters:
//   - c: The number of characters to keep (positive from start, negative from end)
//   - s: The string to truncate
//
// Returns:
//   - string: The truncated string
func trunc(c int, s string) string {
	if c < 0 && len(s)+c > 0 {
		return s[len(s)+c:]
	}
	if c >= 0 && len(s) > c {
		return s[:c]
	}
	return s
}

// title converts a string to title case.
// This uses the English language rules for capitalization.
//
// Parameters:
//   - s: The string to convert
//
// Returns:
//   - string: The string in title case
func title(s string) string {
	return cases.Title(language.English).String(s)
}

// join concatenates the elements of a slice with a separator.
// The input is first converted to a string slice using strslice.
//
// Parameters:
//   - sep: The separator to use between elements
//   - v: The value to join (will be converted to a string slice)
//
// Returns:
//   - string: The joined string
func join(sep string, v any) string {
	return strings.Join(strslice(v), sep)
}

// split splits a string by a separator and returns a map.
// The keys in the map are "_0", "_1", etc., corresponding to the position of each part.
//
// Parameters:
//   - sep: The separator to split on
//   - orig: The string to split
//
// Returns:
//   - map[string]string: A map with keys "_0", "_1", etc. and values being the split parts
func split(sep, orig string) map[string]string {
	parts := strings.Split(orig, sep)
	res := make(map[string]string, len(parts))
	for i, v := range parts {
		res["_"+strconv.Itoa(i)] = v
	}
	return res
}

// splitList splits a string by a separator and returns a slice.
// This is a simple wrapper around strings.Split.
//
// Parameters:
//   - sep: The separator to split on
//   - orig: The string to split
//
// Returns:
//   - []string: A slice containing the split parts
func splitList(sep, orig string) []string {
	return strings.Split(orig, sep)
}

// splitn splits a string by a separator with a limit and returns a map.
// The keys in the map are "_0", "_1", etc., corresponding to the position of each part.
// It will split the string into at most n parts.
//
// Parameters:
//   - sep: The separator to split on
//   - n: The maximum number of parts to return
//   - orig: The string to split
//
// Returns:
//   - map[string]string: A map with keys "_0", "_1", etc. and values being the split parts
func splitn(sep string, n int, orig string) map[string]string {
	parts := strings.SplitN(orig, sep, n)
	res := make(map[string]string, len(parts))
	for i, v := range parts {
		res["_"+strconv.Itoa(i)] = v
	}
	return res
}

// substring creates a substring of the given string.
// It extracts a portion of a string based on start and end indices.
//
// Parameters:
//   - start: The starting index (inclusive)
//   - end: The ending index (exclusive)
//   - s: The source string
//
// Behavior:
//   - If start < 0, returns s[:end]
//   - If start >= 0 and end < 0 or end > len(s), returns s[start:]
//   - Otherwise, returns s[start:end]
//
// Returns:
//   - string: The extracted substring
func substring(start, end int, s string) string {
	if start < 0 {
		return s[:end]
	}
	if end < 0 || end > len(s) {
		return s[start:]
	}
	return s[start:end]
}

// repeat creates a new string by repeating the input string a specified number of times.
// It has safety limits to prevent excessive memory usage or infinite loops.
//
// Parameters:
//   - count: The number of times to repeat the string
//   - str: The string to repeat
//
// Returns:
//   - string: The repeated string
//
// Panics:
//   - If count exceeds loopExecutionLimit
//   - If the resulting string length would exceed stringLengthLimit
func repeat(count int, str string) string {
	if count > loopExecutionLimit {
		panic(fmt.Sprintf("repeat count %d exceeds limit of %d", count, loopExecutionLimit))
	} else if count*len(str) >= stringLengthLimit {
		panic(fmt.Sprintf("repeat count %d with string length %d exceeds limit of %d", count, len(str), stringLengthLimit))
	}
	return strings.Repeat(str, count)
}

// trimAll removes all leading and trailing characters contained in the cutset.
// Note that the parameter order is reversed from the standard strings.Trim function.
//
// Parameters:
//   - a: The cutset of characters to remove
//   - b: The string to trim
//
// Returns:
//   - string: The trimmed string
func trimAll(a, b string) string {
	return strings.Trim(b, a)
}

// trimPrefix removes the specified prefix from a string.
// If the string doesn't start with the prefix, it returns the original string.
// Note that the parameter order is reversed from the standard strings.TrimPrefix function.
//
// Parameters:
//   - a: The prefix to remove
//   - b: The string to trim
//
// Returns:
//   - string: The string with the prefix removed, or the original string if it doesn't start with the prefix
func trimPrefix(a, b string) string {
	return strings.TrimPrefix(b, a)
}

// trimSuffix removes the specified suffix from a string.
// If the string doesn't end with the suffix, it returns the original string.
// Note that the parameter order is reversed from the standard strings.TrimSuffix function.
//
// Parameters:
//   - a: The suffix to remove
//   - b: The string to trim
//
// Returns:
//   - string: The string with the suffix removed, or the original string if it doesn't end with the suffix
func trimSuffix(a, b string) string {
	return strings.TrimSuffix(b, a)
}

// contains checks if a string contains a substring.
//
// Parameters:
//   - substr: The substring to search for
//   - str: The string to search in
//
// Returns:
//   - bool: True if str contains substr, false otherwise
func contains(substr string, str string) bool {
	return strings.Contains(str, substr)
}

// hasPrefix checks if a string starts with a specified prefix.
//
// Parameters:
//   - substr: The prefix to check for
//   - str: The string to check
//
// Returns:
//   - bool: True if str starts with substr, false otherwise
func hasPrefix(substr string, str string) bool {
	return strings.HasPrefix(str, substr)
}

// hasSuffix checks if a string ends with a specified suffix.
//
// Parameters:
//   - substr: The suffix to check for
//   - str: The string to check
//
// Returns:
//   - bool: True if str ends with substr, false otherwise
func hasSuffix(substr string, str string) bool {
	return strings.HasSuffix(str, substr)
}
