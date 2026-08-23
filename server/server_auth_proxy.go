package server

import (
	"errors"
	"net/http"
	"strings"

	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

const (
	proxyUserPasswordLength = 32
	proxyUsernameMaxLength  = 64
)

var errProxyUsernameInvalid = errors.New("invalid username in auth user header")

// authenticateProxyHeader resolves the user named by the auth-user-header, which a trusted reverse
// proxy in front of ntfy is expected to set after it has validated the request itself.
//
// A missing header yields (nil, nil), which lets the caller fall through to the anonymous visitor,
// so that Everyone ACL entries (e.g. write-only access to up* for UnifiedPush) keep working.
func (s *Server) authenticateProxyHeader(r *http.Request) (*user.User, error) {
	username := strings.TrimSpace(r.Header.Get(s.config.AuthUserHeader))
	if username == "" {
		return nil, nil
	}
	if len(username) > proxyUsernameMaxLength || !user.AllowedUsername(username) {
		return nil, errProxyUsernameInvalid
	}
	role, roleAuthoritative := s.proxyHeaderRole(r)
	u, err := s.userManager.User(username)
	if errors.Is(err, user.ErrUserNotFound) && s.config.AuthUserAutoCreate {
		u, err = s.createProxyUser(username, role)
	}
	if err != nil {
		return nil, err
	}
	if u.Deleted {
		return nil, user.ErrUnauthorized
	}
	if roleAuthoritative {
		if err := s.syncProxyUserRole(u, role); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func (s *Server) proxyHeaderRole(r *http.Request) (user.Role, bool) {
	if s.config.AuthGroupsHeader == "" || s.config.AuthAdminGroup == "" {
		return user.RoleUser, false
	}
	for _, group := range strings.Split(r.Header.Get(s.config.AuthGroupsHeader), ",") {
		if strings.TrimSpace(group) == s.config.AuthAdminGroup {
			return user.RoleAdmin, true
		}
	}
	return user.RoleUser, true
}

func (s *Server) createProxyUser(username string, role user.Role) (*user.User, error) {
	password := util.RandomString(proxyUserPasswordLength)
	if err := s.userManager.AddUser(username, password, role, false); err != nil && !errors.Is(err, user.ErrUserExists) {
		return nil, err
	}
	return s.userManager.User(username)
}

func (s *Server) syncProxyUserRole(u *user.User, role user.Role) error {
	if u.Role == role || u.Provisioned {
		return nil
	}
	if err := s.userManager.ChangeRole(u.Name, role); err != nil {
		return err
	}
	u.Role = role
	return nil
}
