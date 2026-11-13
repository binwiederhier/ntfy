package sprig

import (
	"regexp"
	"strings"
)

// regexMatch checks if a string matches a regular expression pattern.
// It ignores any errors that might occur during regex compilation.
//
// Parameters:
//   - regex: The regular expression pattern to match against
//   - s: The string to check
//
// Returns:
//   - bool: True if the string matches the pattern, false otherwise
func regexMatch(regex string, s string) bool {
	match, _ := regexp.MatchString(regex, s)
	return match
}

// mustRegexMatch checks if a string matches a regular expression pattern.
// Unlike regexMatch, this function returns any errors that occur during regex compilation.
//
// Parameters:
//   - regex: The regular expression pattern to match against
//   - s: The string to check
//
// Returns:
//   - bool: True if the string matches the pattern, false otherwise
//   - error: Any error that occurred during regex compilation
func mustRegexMatch(regex string, s string) (bool, error) {
	return regexp.MatchString(regex, s)
}

// regexFindAll finds all matches of a regular expression in a string.
// It panics if the regex pattern cannot be compiled.
//
// Parameters:
//   - regex: The regular expression pattern to search for
//   - s: The string to search within
//   - n: The maximum number of matches to return (negative means all matches)
//
// Returns:
//   - []string: A slice containing all matched substrings
func regexFindAll(regex string, s string, n int) []string {
	r := regexp.MustCompile(regex)
	return r.FindAllString(s, n)
}

// mustRegexFindAll finds all matches of a regular expression in a string.
// Unlike regexFindAll, this function returns any errors that occur during regex compilation.
//
// Parameters:
//   - regex: The regular expression pattern to search for
//   - s: The string to search within
//   - n: The maximum number of matches to return (negative means all matches)
//
// Returns:
//   - []string: A slice containing all matched substrings
//   - error: Any error that occurred during regex compilation
func mustRegexFindAll(regex string, s string, n int) ([]string, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return []string{}, err
	}
	return r.FindAllString(s, n), nil
}

// regexFind finds the first match of a regular expression in a string.
// It panics if the regex pattern cannot be compiled.
//
// Parameters:
//   - regex: The regular expression pattern to search for
//   - s: The string to search within
//
// Returns:
//   - string: The first matched substring, or an empty string if no match
func regexFind(regex string, s string) string {
	r := regexp.MustCompile(regex)
	return r.FindString(s)
}

// mustRegexFind finds the first match of a regular expression in a string.
// Unlike regexFind, this function returns any errors that occur during regex compilation.
//
// Parameters:
//   - regex: The regular expression pattern to search for
//   - s: The string to search within
//
// Returns:
//   - string: The first matched substring, or an empty string if no match
//   - error: Any error that occurred during regex compilation
func mustRegexFind(regex string, s string) (string, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return "", err
	}
	return r.FindString(s), nil
}

// regexReplaceAll replaces all matches of a regular expression with a replacement string.
// It panics if the regex pattern cannot be compiled.
// The replacement string can contain $1, $2, etc. for submatches.
//
// Parameters:
//   - regex: The regular expression pattern to search for
//   - s: The string to search within
//   - repl: The replacement string (can contain $1, $2, etc. for submatches)
//
// Returns:
//   - string: The resulting string after all replacements
func regexReplaceAll(regex string, s string, repl string) string {
	r := regexp.MustCompile(regex)
	checkResultLength(regexReplaceResultLen(r, s, repl, false))
	return r.ReplaceAllString(s, repl)
}

// mustRegexReplaceAll replaces all matches of a regular expression with a replacement string.
// Unlike regexReplaceAll, this function returns any errors that occur during regex compilation.
// The replacement string can contain $1, $2, etc. for submatches.
//
// Parameters:
//   - regex: The regular expression pattern to search for
//   - s: The string to search within
//   - repl: The replacement string (can contain $1, $2, etc. for submatches)
//
// Returns:
//   - string: The resulting string after all replacements
//   - error: Any error that occurred during regex compilation
func mustRegexReplaceAll(regex string, s string, repl string) (string, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return "", err
	}
	checkResultLength(regexReplaceResultLen(r, s, repl, false))
	return r.ReplaceAllString(s, repl), nil
}

// regexReplaceAllLiteral replaces all matches of a regular expression with a literal replacement string.
// It panics if the regex pattern cannot be compiled.
// Unlike regexReplaceAll, the replacement string is used literally (no $1, $2 processing).
//
// Parameters:
//   - regex: The regular expression pattern to search for
//   - s: The string to search within
//   - repl: The literal replacement string
//
// Returns:
//   - string: The resulting string after all replacements
func regexReplaceAllLiteral(regex string, s string, repl string) string {
	r := regexp.MustCompile(regex)
	checkResultLength(regexReplaceResultLen(r, s, repl, true))
	return r.ReplaceAllLiteralString(s, repl)
}

// mustRegexReplaceAllLiteral replaces all matches of a regular expression with a literal replacement string.
// Unlike regexReplaceAllLiteral, this function returns any errors that occur during regex compilation.
// The replacement string is used literally (no $1, $2 processing).
//
// Parameters:
//   - regex: The regular expression pattern to search for
//   - s: The string to search within
//   - repl: The literal replacement string
//
// Returns:
//   - string: The resulting string after all replacements
//   - error: Any error that occurred during regex compilation
func mustRegexReplaceAllLiteral(regex string, s string, repl string) (string, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return "", err
	}
	checkResultLength(regexReplaceResultLen(r, s, repl, true))
	return r.ReplaceAllLiteralString(s, repl), nil
}

// regexSplit splits a string by a regular expression pattern.
// It panics if the regex pattern cannot be compiled.
//
// Parameters:
//   - regex: The regular expression pattern to split on
//   - s: The string to split
//   - n: The maximum number of substrings to return (negative means all substrings)
//
// Returns:
//   - []string: A slice containing the substrings between regex matches
func regexSplit(regex string, s string, n int) []string {
	r := regexp.MustCompile(regex)
	return r.Split(s, n)
}

// mustRegexSplit splits a string by a regular expression pattern.
// Unlike regexSplit, this function returns any errors that occur during regex compilation.
//
// Parameters:
//   - regex: The regular expression pattern to split on
//   - s: The string to split
//   - n: The maximum number of substrings to return (negative means all substrings)
//
// Returns:
//   - []string: A slice containing the substrings between regex matches
//   - error: Any error that occurred during regex compilation
func mustRegexSplit(regex string, s string, n int) ([]string, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return []string{}, err
	}
	return r.Split(s, n), nil
}

// regexQuoteMeta escapes all regular expression metacharacters in a string.
// This is useful when you want to use a string as a literal in a regular expression.
//
// Parameters:
//   - s: The string to escape
//
// Returns:
//   - string: The escaped string with all regex metacharacters quoted
func regexQuoteMeta(s string) string {
	return regexp.QuoteMeta(s)
}

// regexReplaceResultLen returns an upper bound on the size of r.ReplaceAllString(s, repl) (or the
// literal form), so a caller can reject an oversized result before building it. Each of the up to
// len(s)+1 matches emits repl, and in the non-literal form every "$" reference in repl expands to
// at most len(s) bytes. It never under-estimates, so it never lets an oversized replace through.
func regexReplaceResultLen(r *regexp.Regexp, s, repl string, literal bool) int {
	matches := len(r.FindAllStringIndex(s, -1))
	perMatch := len(repl)
	if !literal {
		perMatch += strings.Count(repl, "$") * len(s)
	}
	return len(s) + matches*perMatch
}
