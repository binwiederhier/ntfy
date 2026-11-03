//go:build !noserver && windows

package cmd

// checkAdminSocketOwner does nothing on Windows, where there is no admin socket by default
func checkAdminSocketOwner(socket, authFile string) error {
	return nil
}
