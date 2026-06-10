//go:build windows

package tray

import (
	"strings"
	"testing"
)

func TestToastXML_EscapesNotificationContent(t *testing.T) {
	xml, err := toastXML(Notification{
		Title:   `$(Start-Process calc) <title>`,
		Message: `message & "quoted"`,
	}, `C:\Program Files\ntfy\favicon.ico`)
	if err != nil {
		t.Fatalf("toastXML() error = %v", err)
	}

	for _, disallowed := range []string{`<title>`, `message & "quoted"`} {
		if strings.Contains(xml, disallowed) {
			t.Fatalf("toastXML() contains unescaped content %q in %q", disallowed, xml)
		}
	}
	for _, expected := range []string{`$(Start-Process calc) &lt;title&gt;`, `message &amp; &#34;quoted&#34;`, `C:\Program Files\ntfy\favicon.ico`} {
		if !strings.Contains(xml, expected) {
			t.Fatalf("toastXML() = %q, want substring %q", xml, expected)
		}
	}
}

func TestToastScript_DoesNotEmbedNotificationContent(t *testing.T) {
	xml, err := toastXML(Notification{
		Title:   `$(Start-Process calc)`,
		Message: `hello`,
	}, "")
	if err != nil {
		t.Fatalf("toastXML() error = %v", err)
	}

	script, err := renderToastScript(xml)
	if err != nil {
		t.Fatalf("renderToastScript() error = %v", err)
	}
	if strings.Contains(script, `$(Start-Process calc)`) || strings.Contains(script, "hello") {
		t.Fatalf("rendered script contains notification content: %q", script)
	}
}
