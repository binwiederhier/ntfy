//go:build windows

package tray

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"os/exec"
	"path/filepath"
	"syscall"
	"text/template"
	"time"
)

const (
	toastAppID       = "ntfy"
	toastPushTimeout = 10 * time.Second
)

var toastScript = template.Must(template.New("toastScript").Parse(`
$encodedXml = '{{.EncodedXML}}'
$APP_ID = '{{.AppID}}'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$xmlString = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($encodedXml))
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($xmlString)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($APP_ID).Show($toast)
`))

// ToastNotifier displays Windows toast notifications.
type ToastNotifier struct {
	icon string
}

// NewToastNotifier creates a Windows toast notifier.
func NewToastNotifier(icon string) *ToastNotifier {
	if icon != "" {
		if abs, err := filepath.Abs(icon); err == nil {
			icon = abs
		}
	}
	return &ToastNotifier{icon: icon}
}

// Notify displays a Windows toast notification.
func (n *ToastNotifier) Notify(notification Notification) error {
	xml, err := toastXML(notification, n.icon)
	if err != nil {
		return err
	}
	script, err := renderToastScript(xml)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), toastPushTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "PowerShell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-EncodedCommand", base64.StdEncoding.EncodeToString(utf16LE(script)))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func renderToastScript(xml string) (string, error) {
	var script bytes.Buffer
	err := toastScript.Execute(&script, map[string]string{
		"AppID":      toastAppID,
		"EncodedXML": base64.StdEncoding.EncodeToString([]byte(xml)),
	})
	if err != nil {
		return "", err
	}
	return script.String(), nil
}

func toastXML(notification Notification, icon string) (string, error) {
	var xmlBuffer bytes.Buffer
	xmlBuffer.WriteString(`<toast duration="short"><visual><binding template="ToastGeneric">`)
	xmlBuffer.WriteString(`<text>`)
	if err := xml.EscapeText(&xmlBuffer, []byte(notification.Title)); err != nil {
		return "", err
	}
	xmlBuffer.WriteString(`</text><text>`)
	if err := xml.EscapeText(&xmlBuffer, []byte(notification.Message)); err != nil {
		return "", err
	}
	xmlBuffer.WriteString(`</text>`)
	if icon != "" {
		xmlBuffer.WriteString(`<image placement="appLogoOverride" src="`)
		if err := xml.EscapeText(&xmlBuffer, []byte(icon)); err != nil {
			return "", err
		}
		xmlBuffer.WriteString(`"/>`)
	}
	xmlBuffer.WriteString(`</binding></visual><audio src="ms-winsoundevent:Notification.Default"/></toast>`)
	return xmlBuffer.String(), nil
}

func utf16LE(value string) []byte {
	out := make([]byte, 0, len(value)*2)
	for _, r := range value {
		if r > 0xffff {
			r -= 0x10000
			high := uint16(0xd800 + (r >> 10))
			low := uint16(0xdc00 + (r & 0x3ff))
			out = append(out, byte(high), byte(high>>8), byte(low), byte(low>>8))
			continue
		}
		u := uint16(r)
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}
