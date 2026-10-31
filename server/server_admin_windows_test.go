//go:build windows

package server

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestAdminSocket_NewDirectoryDACL(t *testing.T) {
	conf := newTestConfigWithAuthFile(t, "")
	conf.ListenAdminUnix = filepath.Join(shortTempDir(t), "sub", "admin.sock")
	s := newTestServer(t, conf)
	runTestServer(t, s)
	waitFor(t, func() bool { return adminSocketListening(s) })

	// Only Administrators, SYSTEM and the server's user, on the directory and the socket
	dir := filepath.Dir(conf.ListenAdminUnix)
	require.True(t, isDACLProtected(t, dir))
	require.Equal(t, adminSocketAllowedSIDs(t), daclSIDs(t, dir))
	require.Equal(t, adminSocketAllowedSIDs(t), daclSIDs(t, conf.ListenAdminUnix))
}

func TestAdminSocket_ExistingDirectoryDACL(t *testing.T) {
	// An existing directory (e.g. %ProgramData%\ntfy, which holds server.yml) inherits whatever
	// its parent grants; it is locked down, but the files already in it are not touched
	dir := shortTempDir(t)
	other := filepath.Join(dir, "server.yml")
	require.Nil(t, os.WriteFile(other, []byte("x"), 0600))
	otherBefore := daclString(t, other)
	conf := newTestConfigWithAuthFile(t, "")
	conf.ListenAdminUnix = filepath.Join(dir, "admin.sock")
	s := newTestServer(t, conf)
	runTestServer(t, s)
	waitFor(t, func() bool { return adminSocketListening(s) })
	require.True(t, isDACLProtected(t, dir))
	require.Equal(t, adminSocketAllowedSIDs(t), daclSIDs(t, dir))
	require.Equal(t, adminSocketAllowedSIDs(t), daclSIDs(t, conf.ListenAdminUnix))
	require.Equal(t, otherBefore, daclString(t, other))
}

func TestAdminSocket_RemovedOnStopWindows(t *testing.T) {
	s, socket := newTestServerWithAdminSocket(t, newTestConfigWithAuthFile(t, ""))
	resp, err := adminSocketClient(socket).Get("http://ntfy/v1/users")
	require.Nil(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	s.Stop()
	_, err = os.Lstat(socket)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// adminSocketAllowedSIDs returns the sorted SIDs that may open the admin socket
func adminSocketAllowedSIDs(t *testing.T) []string {
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	require.Nil(t, err)
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	require.Nil(t, err)
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.Nil(t, err)
	sids := []string{admins.String(), system.String(), tokenUser.User.Sid.String()}
	slices.Sort(sids)
	return slices.Compact(sids) // The test may run as SYSTEM
}

// daclSIDs returns the sorted, distinct SIDs of all allow entries in the DACL of path
func daclSIDs(t *testing.T, path string) []string {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.Nil(t, err)
	dacl, _, err := sd.DACL()
	require.Nil(t, err)
	require.NotNil(t, dacl)
	sids := make([]string, 0)
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		require.Nil(t, windows.GetAce(dacl, uint32(i), &ace))
		require.Equal(t, uint8(windows.ACCESS_ALLOWED_ACE_TYPE), ace.Header.AceType)
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		sids = append(sids, sid.String())
	}
	slices.Sort(sids)
	return slices.Compact(sids)
}

func isDACLProtected(t *testing.T, path string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.Nil(t, err)
	control, _, err := sd.Control()
	require.Nil(t, err)
	return control&windows.SE_DACL_PROTECTED != 0
}

func daclString(t *testing.T, path string) string {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.Nil(t, err)
	return sd.String()
}
