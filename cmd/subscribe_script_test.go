package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteBatchScriptVars_MessageVariables(t *testing.T) {
	script := `notifu /m "%NTFY_MESSAGE%"`
	expected := `notifu /m "!NTFY_MESSAGE!"`
	require.Equal(t, expected, rewriteBatchScriptVars(script))
}

func TestRewriteBatchScriptVars_Aliases_CaseInsensitive(t *testing.T) {
	script := `echo Message received: %message%, %M%, %Ntfy_Message%, %PRIORITY%`
	expected := `echo Message received: !message!, !M!, !Ntfy_Message!, !PRIORITY!`
	require.Equal(t, expected, rewriteBatchScriptVars(script))
}

func TestRewriteBatchScriptVars_LeavesOtherVariablesAlone(t *testing.T) {
	script := `echo %PATH% && %UNKNOWN_VAR% 100%%`
	require.Equal(t, script, rewriteBatchScriptVars(script))
}

func TestRewriteBatchScriptVars_LeavesSubstringExpansionAlone(t *testing.T) {
	script := `echo %NTFY_MESSAGE:~0,5%`
	require.Equal(t, script, rewriteBatchScriptVars(script))
}

func TestRewriteBatchScriptVars_NormalizesLineEndings(t *testing.T) {
	script := "echo one\necho two\r\necho three"
	expected := "echo one\r\necho two\r\necho three"
	require.Equal(t, expected, rewriteBatchScriptVars(script))
}

func TestRewriteBatchScriptVars_AllDocumentedVariables(t *testing.T) {
	script := `%NTFY_ID% %NTFY_TIME% %NTFY_TOPIC% %NTFY_MESSAGE% %NTFY_TITLE% %NTFY_PRIORITY% %NTFY_TAGS% %NTFY_RAW% %id% %time% %topic% %message% %m% %title% %t% %priority% %prio% %p% %tags% %tag% %ta% %raw%`
	expected := `!NTFY_ID! !NTFY_TIME! !NTFY_TOPIC! !NTFY_MESSAGE! !NTFY_TITLE! !NTFY_PRIORITY! !NTFY_TAGS! !NTFY_RAW! !id! !time! !topic! !message! !m! !title! !t! !priority! !prio! !p! !tags! !tag! !ta! !raw!`
	require.Equal(t, expected, rewriteBatchScriptVars(script))
}
