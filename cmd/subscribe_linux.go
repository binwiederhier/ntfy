package cmd

const (
	scriptExt                      = "sh"
	scriptHeader                   = "#!/bin/sh\n"
	clientCommandDescriptionSuffix = `The default config file for all client commands is /etc/ntfy/client.yml (if root user),
or ~/.config/ntfy/client.yml for all other users.`
)

var (
	scriptLauncher = []string{"sh", "-c"}
)

func defaultClientConfigFile() string {
	return defaultClientConfigFileUnix()
}
