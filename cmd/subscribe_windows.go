//go:build windows

package cmd

const (
	scriptExt                      = "bat"
	scriptHeader                   = "@echo off\r\nsetlocal EnableExtensions EnableDelayedExpansion\r\n"
	clientCommandDescriptionSuffix = `The default config file for all client commands is %AppData%\ntfy\client.yml.`
)

var (
	scriptLauncher    = []string{"cmd.exe", "/Q", "/C"}
	scriptVarRewriter = rewriteBatchScriptVars
)
