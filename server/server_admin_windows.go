//go:build windows

package server

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

const (
	// adminSocketDirSDDL is the DACL of the socket's directory: full access for Administrators,
	// SYSTEM and the user the server runs as (%s), inherited by files in it, nothing inherited
	// from the parent (P). File modes mean nothing on Windows, so this is what keeps everyone
	// else from opening the socket.
	adminSocketDirSDDL = "D:P(A;OICI;FA;;;BA)(A;OICI;FA;;;SY)(A;OICI;FA;;;%s)"
)

// prepareAdminSocketDir creates the socket's directory and locks it down with adminSocketDirSDDL,
// also if it exists: the default %ProgramData%\ntfy inherits from %ProgramData% that any user may
// create files in it, so anyone could plant a fake admin.sock there. Only the directory itself is
// changed (SetKernelObjectSecurity, unlike SetNamedSecurityInfo, does not propagate to children),
// so files already in it keep their permissions, and a misconfigured path cannot rewrite the
// permissions of a whole tree.
func prepareAdminSocketDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf(adminSocketDirSDDL, tokenUser.User.Sid.String()))
	if err != nil {
		return err
	}
	dirPtr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(dirPtr, windows.WRITE_DAC|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return fmt.Errorf("cannot open %s to restrict access: %w", dir, err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.SetKernelObjectSecurity(handle, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, sd); err != nil {
		return fmt.Errorf("cannot restrict access to %s: %w", dir, err)
	}
	return nil
}

// restrictAdminSocket does nothing on Windows: the socket inherits the directory's DACL when it is
// created, see prepareAdminSocketDir
func restrictAdminSocket(path string) error {
	return nil
}
