package cmd

import (
	"regexp"
	"strings"
)

// scriptVarsPattern matches the ntfy message variables in their cmd.exe
// percent-expansion syntax, e.g. %NTFY_MESSAGE% or %m% (case-insensitive).
// Only exact variable references are matched: substring and partial
// expansions such as %NTFY_MESSAGE:~0,5% are intentionally left untouched,
// and so are all other environment variables like %PATH%.
var scriptVarsPattern = regexp.MustCompile(`(?i)%(NTFY_ID|NTFY_TIME|NTFY_TOPIC|NTFY_MESSAGE|NTFY_TITLE|NTFY_PRIORITY|NTFY_TAGS|NTFY_RAW|id|time|topic|message|m|title|t|priority|prio|p|tags|tag|ta|raw)%`)

// rewriteBatchScriptVars prepares a user command for execution as a Windows
// batch script. cmd.exe substitutes %VAR% references with the raw variable
// value before the line is parsed, so message content containing & | > or
// quotes would be executed as command syntax (see
// https://github.com/binwiederhier/ntfy/issues/1721). Rewriting the
// references to delayed expansion (!VAR!) moves the substitution to after
// the parse step, which turns message content into inert data.
//
// Note that with delayed expansion enabled, a literal ! in a command must
// be escaped as ^^!.
func rewriteBatchScriptVars(script string) string {
	script = strings.ReplaceAll(script, "\r\n", "\n")
	script = strings.ReplaceAll(script, "\n", "\r\n")
	return scriptVarsPattern.ReplaceAllString(script, "!$1!")
}
