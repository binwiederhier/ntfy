//go:build !windows

package server

func init() {
	DefaultConfigFile = "/etc/ntfy/server.yml"
	DefaultTemplateDir = "/etc/ntfy/templates"
}
