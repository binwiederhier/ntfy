package cmd

const (
	scriptExt                      = "sh"
	scriptHeader                   = "#!/bin/sh\n"
	clientCommandDescriptionSuffix = `The default config file for all client commands is /etc/ntfy/client.yml (if root user),
or "~/Library/Application Support" for all other users.`
)

var (
	scriptLauncher = []string{"sh", "-c"}
)

func defaultConfigFile() string {
	return defaultConfigFileUnix()
}
