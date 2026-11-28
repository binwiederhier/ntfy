package server

// insecureAdminSocketDir is not implemented on Windows, where file modes do not apply
func insecureAdminSocketDir(dir string) string {
	return ""
}
