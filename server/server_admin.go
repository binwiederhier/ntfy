package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"
)

const (
	apiTierReset           = "-" // Tier value that removes a user's tier (PUT /v1/users); same marker the CLI uses
	adminSocketDirMode     = fs.FileMode(0750)
	adminSocketDialTimeout = time.Second // Used to detect a stale socket file (nobody listening)
)

var (
	// adminSocketPaths are the paths served over the admin socket. All of them are admin-only
	// routes in handleInternal (ensureAdmin), which the socket's synthetic admin user passes.
	adminSocketPaths = []string{
		apiVersionPath,
		apiUsersPath,
		apiUsersAccessPath,
		apiUsersTokensPath,
		apiUsersPasswordResetPath,
		apiTiersPath,
	}

	// adminSocketUser is the synthetic admin every socket request runs as. Its name is one that
	// user.AllowedUsername rejects, so it can never collide with a real account.
	adminSocketUser = &user.User{
		ID:      "u_admin_socket",
		Name:    "(admin socket)",
		Role:    user.RoleAdmin,
		Stats:   &user.Stats{},   // Read by newVisitor
		Billing: &user.Billing{}, // Read by the visitor's log context
	}

	// adminSocketIP is the visitor IP of every socket request; they all come from this machine
	adminSocketIP = netip.AddrFrom4([4]byte{127, 0, 0, 1})
)

// DefaultListenAdminUnix returns the default admin socket path. On Windows there is none: unix socket
// file permissions do not protect it there, so the CLI uses the database or --server instead.
func DefaultListenAdminUnix() string {
	return defaultListenAdminUnixFor(runtime.GOOS)
}

func defaultListenAdminUnixFor(goos string) string {
	switch goos {
	case "windows":
		return ""
	case "linux":
		return "/run/ntfy/admin.sock"
	}
	return "/var/run/ntfy/admin.sock" // macOS and the BSDs have no /run
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request, v *visitor) error {
	return s.writeJSON(w, &apiVersionResponse{
		Version: s.config.BuildVersion,
		Commit:  s.config.BuildCommit,
		Date:    s.config.BuildDate,
	})
}

func (s *Server) handleUsersGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	users, err := s.userManager.Users()
	if err != nil {
		return err
	}
	grants, err := s.userManager.AllGrants()
	if err != nil {
		return err
	}
	usersResponse := make([]*apiUserResponse, len(users))
	for i, u := range users {
		userGrants := make([]*apiUserGrantResponse, len(grants[u.ID]))
		for i, g := range grants[u.ID] {
			userGrants[i] = &apiUserGrantResponse{
				Topic:       g.TopicPattern,
				Permission:  g.Permission.String(),
				Provisioned: g.Provisioned,
			}
		}
		usersResponse[i] = &apiUserResponse{
			Username:    u.Name,
			Role:        string(u.Role),
			Provisioned: u.Provisioned,
			Grants:      userGrants,
		}
		if u.Tier != nil {
			usersResponse[i].Tier = u.Tier.Code
			usersResponse[i].TierName = u.Tier.Name
		}
		if u.Name == user.Everyone {
			usersResponse[i].DefaultAccess = s.userManager.DefaultAccess().String()
		}
	}
	return s.writeJSON(w, usersResponse)
}

func (s *Server) handleUsersAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUserAddOrUpdateRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !user.AllowedUsername(req.Username) || (req.Password == "" && req.Hash == "") {
		return errHTTPBadRequest.Wrap("username invalid, or \"password\" or \"hash\" missing")
	}
	role := user.RoleUser
	if req.Role != "" {
		if role, err = checkRoleChange(r, req.Role); err != nil {
			return err
		}
	}
	u, err := s.userManager.User(req.Username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return err
	} else if u != nil {
		return errHTTPConflictUserExists
	}
	if req.Tier != "" {
		if _, err := s.userManager.Tier(req.Tier); err != nil {
			return mapUserErrorToHTTP(err)
		}
	}
	password, hashed := req.Password, false
	if req.Hash != "" {
		password, hashed = req.Hash, true
	}
	adminLog(v, r).Info("Adding user %s (role %s)", req.Username, role)
	if err := s.userManager.AddUser(req.Username, password, role, hashed); err != nil {
		return mapUserErrorToHTTP(err)
	}
	if req.Tier != "" {
		if err := s.userManager.ChangeTier(req.Username, req.Tier); err != nil {
			return mapUserErrorToHTTP(err)
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleUsersUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUserAddOrUpdateRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !user.AllowedUsername(req.Username) {
		return errHTTPBadRequest.Wrap("username invalid")
	} else if req.Password == "" && req.Hash == "" && req.Tier == "" && req.Role == "" {
		return errHTTPBadRequest.Wrap("need to provide at least one of \"password\", \"hash\", \"tier\" or \"role\"")
	}
	var role user.Role
	if req.Role != "" {
		if role, err = checkRoleChange(r, req.Role); err != nil {
			return err
		}
	}
	if req.Tier != "" && req.Tier != apiTierReset {
		if _, err := s.userManager.Tier(req.Tier); err != nil {
			return mapUserErrorToHTTP(err)
		}
	}
	u, err := s.userManager.User(req.Username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return err
	} else if u != nil {
		if u.IsAdmin() && !isAdminSocketRequest(r) {
			return errHTTPForbiddenAdminSocketOnly
		}
		if req.Hash != "" || req.Password != "" {
			adminLog(v, r).Info("Changing password of user %s", req.Username)
			if req.Hash != "" {
				err = s.userManager.ChangePassword(req.Username, req.Hash, true)
			} else {
				err = s.userManager.ChangePassword(req.Username, req.Password, false)
			}
			if err != nil {
				return mapUserErrorToHTTP(err)
			}
		}
	} else if req.Role != "" {
		return errHTTPBadRequestUserNotFound // Role changes only apply to existing users, see apiUserAddOrUpdateRequest
	} else {
		// Upsert: the admin UI creates users with PUT
		password, hashed := req.Password, false
		if req.Hash != "" {
			password, hashed = req.Hash, true
		}
		adminLog(v, r).Info("Adding user %s (role %s)", req.Username, user.RoleUser)
		if err := s.userManager.AddUser(req.Username, password, user.RoleUser, hashed); err != nil {
			return mapUserErrorToHTTP(err)
		}
	}
	if req.Tier == apiTierReset {
		adminLog(v, r).Info("Removing tier of user %s", req.Username)
		if err := s.userManager.ResetTier(req.Username); err != nil {
			return mapUserErrorToHTTP(err)
		}
	} else if req.Tier != "" {
		adminLog(v, r).Info("Changing tier of user %s to %s", req.Username, req.Tier)
		if err := s.userManager.ChangeTier(req.Username, req.Tier); err != nil {
			return mapUserErrorToHTTP(err)
		}
	}
	if req.Role != "" {
		adminLog(v, r).Info("Changing role of user %s to %s", req.Username, role)
		if err := s.userManager.ChangeRole(req.Username, role); err != nil {
			return mapUserErrorToHTTP(err)
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleUsersDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUserDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if err != nil {
		return mapUserErrorToHTTP(err)
	} else if u.Name == user.Everyone {
		return errHTTPBadRequest.Wrap("cannot remove the everyone user")
	} else if !u.IsUser() && !isAdminSocketRequest(r) {
		return errHTTPForbiddenAdminSocketOnly
	}
	adminLog(v, r).Info("Removing user %s", req.Username)
	if err := s.userManager.RemoveUser(req.Username); err != nil {
		return mapUserErrorToHTTP(err)
	}
	if s.webPush != nil {
		if err := s.webPush.RemoveSubscriptionsByUserID(u.ID); err != nil {
			adminLog(v, r).Err(err).Warn("Failed to remove web push subscriptions of user %s", req.Username)
		}
	}
	s.userDeleted(u) // FIXME super inefficient
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccessAllow(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccessAllowRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if err != nil {
		return mapUserErrorToHTTP(err)
	} else if u.IsAdmin() {
		return errHTTPBadRequestAccessAdminUser
	}
	permission, err := user.ParsePermission(req.Permission)
	if err != nil {
		return errHTTPBadRequestPermissionInvalid
	}
	adminLog(v, r).Info("Granting %s access to topic %s for user %s", permission, req.Topic, req.Username)
	if err := s.userManager.AllowAccess(req.Username, req.Topic, permission); err != nil {
		return mapUserErrorToHTTP(err)
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccessReset(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccessResetRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if req.Username == "" && req.Topic != "" {
		return errHTTPBadRequest.Wrap("topic requires a username")
	} else if req.Username == "" && !isAdminSocketRequest(r) {
		return errHTTPForbiddenAdminSocketOnly // Resetting the whole ACL also opens all reserved topics
	}
	var u *user.User
	if req.Username != "" {
		if u, err = s.userManager.User(req.Username); err != nil {
			return mapUserErrorToHTTP(err)
		}
	}
	adminLog(v, r).Info("Resetting access for user %s, topic %s", req.Username, req.Topic)
	if err := s.userManager.ResetAccess(req.Username, req.Topic); err != nil {
		return mapUserErrorToHTTP(err)
	}
	// Without a username, the whole ACL is gone. Subscribers are not killed here; the periodic
	// re-authorization of long-lived subscriptions catches up with them.
	if u != nil {
		topicPattern := req.Topic
		if topicPattern == "" {
			topicPattern = "*"
		}
		if err := s.killUserSubscriber(u, topicPattern); err != nil { // This may be a pattern
			return err
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleUsersTokensGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	var users []*user.User
	if username := r.URL.Query().Get("username"); username != "" {
		u, err := s.userManager.User(username)
		if err != nil {
			return mapUserErrorToHTTP(err)
		} else if u.IsAdmin() && !isAdminSocketRequest(r) {
			return errHTTPForbiddenAdminSocketOnly // Admin users' tokens can only be managed over the admin socket
		}
		users = []*user.User{u}
	} else if !isAdminSocketRequest(r) {
		return errHTTPForbiddenAdminSocketOnly // All users' tokens in one call, too much for a leaked admin token
	} else {
		var err error
		if users, err = s.userManager.Users(); err != nil {
			return err
		}
	}
	response := make([]*apiUserTokenResponse, 0)
	for _, u := range users {
		tokens, err := s.userManager.Tokens(u.ID)
		if err != nil {
			return err
		}
		for _, t := range tokens {
			response = append(response, newAPIUserTokenResponse(u.Name, t))
		}
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleUsersTokensAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUsersTokensAddRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if req.Username == user.Everyone {
		return errHTTPBadRequest.Wrap("cannot create tokens for the everyone user")
	}
	u, err := s.userManager.User(req.Username)
	if err != nil {
		return mapUserErrorToHTTP(err)
	} else if u.IsAdmin() && !isAdminSocketRequest(r) {
		return errHTTPForbiddenAdminSocketOnly // Admin users' tokens can only be managed over the admin socket
	}
	expires := time.Unix(0, 0)
	if req.Expires != 0 {
		expires = time.Unix(req.Expires, 0)
		if !expires.After(time.Now()) {
			return errHTTPBadRequest.Wrap("expires must be in the future")
		}
	}
	adminLog(v, r).Info("Creating token for user %s", req.Username)
	token, err := s.userManager.CreateToken(u.ID, req.Label, expires, netip.IPv4Unspecified(), false)
	if err != nil {
		return mapUserErrorToHTTP(err)
	}
	return s.writeJSON(w, newAPIUserTokenResponse(u.Name, token))
}

func (s *Server) handleUsersTokensDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUsersTokensDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if err != nil {
		return mapUserErrorToHTTP(err)
	} else if u.IsAdmin() && !isAdminSocketRequest(r) {
		return errHTTPForbiddenAdminSocketOnly // Admin users' tokens can only be managed over the admin socket
	}
	adminLog(v, r).Info("Removing token for user %s", req.Username)
	if err := s.userManager.RemoveToken(u.ID, req.Token); err != nil {
		return mapUserErrorToHTTP(err)
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleUsersPasswordReset(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUsersPasswordResetRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if s.config.BaseURL == "" {
		return errHTTPBadRequestBaseURLNotConfigured
	}
	u, err := s.userManager.User(req.Username)
	if err != nil {
		return mapUserErrorToHTTP(err)
	} else if u.Provisioned {
		return errHTTPConflictProvisionedUserChange
	} else if u.IsAdmin() && !isAdminSocketRequest(r) {
		return errHTTPForbiddenAdminSocketOnly
	}
	var email string
	if req.SendEmail {
		if s.mailer == nil {
			return errHTTPBadRequest.Wrap("smtp-sender-addr is not configured on the server")
		}
		if email, err = s.userManager.PrimaryEmail(u.ID); err != nil {
			return err
		} else if email == "" {
			return errHTTPBadRequestUserNoPrimaryEmail
		}
	}
	token, err := s.userManager.AddMagicLink(user.MagicLinkKindPasswordReset, u.ID, "", passwordResetTokenExpiry)
	if err != nil {
		return err
	}
	link := s.config.BaseURL + webAppPasswordResetPathPrefix + token
	adminLog(v, r).Info("Creating password reset link for user %s", req.Username)
	if req.SendEmail {
		if err := s.mailer.SendPasswordReset(email, link); err != nil {
			adminLog(v, r).Err(err).Warn("Failed to send password reset email to %s", email)
			return fmt.Errorf("failed to send reset email to %s: %w", email, err)
		}
	}
	return s.writeJSON(w, &apiUsersPasswordResetResponse{
		Link:  link,
		Email: email,
	})
}

func (s *Server) handleAdminTiersGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	tiers, err := s.userManager.Tiers()
	if err != nil {
		return err
	}
	counts, err := s.userManager.TierUsersCount()
	if err != nil {
		return err
	}
	slices.SortFunc(tiers, func(a, b *user.Tier) int {
		return strings.Compare(a.Code, b.Code)
	})
	response := make([]*apiAdminTier, len(tiers))
	for i, t := range tiers {
		response[i] = &apiAdminTier{
			ID:                       t.ID,
			Code:                     t.Code,
			Name:                     t.Name,
			MessagesLimit:            t.MessageLimit,
			MessagesExpiryDuration:   int64(t.MessageExpiryDuration.Seconds()),
			EmailsLimit:              t.EmailLimit,
			CallsLimit:               t.CallLimit,
			ReservationsLimit:        t.ReservationLimit,
			AttachmentFileSizeLimit:  t.AttachmentFileSizeLimit,
			AttachmentTotalSizeLimit: t.AttachmentTotalSizeLimit,
			AttachmentExpiryDuration: int64(t.AttachmentExpiryDuration.Seconds()),
			AttachmentBandwidthLimit: t.AttachmentBandwidthLimit,
			StripeMonthlyPriceID:     t.StripeMonthlyPriceID,
			StripeYearlyPriceID:      t.StripeYearlyPriceID,
			Users:                    counts[t.Code],
		}
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleAdminTiersAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	tier, err := readAdminTierRequest(r)
	if err != nil {
		return err
	}
	if _, err := s.userManager.Tier(tier.Code); err == nil {
		return errHTTPConflictTierExists
	} else if !errors.Is(err, user.ErrTierNotFound) {
		return err
	}
	adminLog(v, r).Info("Adding tier %s", tier.Code)
	if err := s.userManager.AddTier(tier); err != nil {
		return mapUserErrorToHTTP(err)
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAdminTiersUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	tier, err := readAdminTierRequest(r)
	if err != nil {
		return err
	}
	if _, err := s.userManager.Tier(tier.Code); err != nil {
		return mapUserErrorToHTTP(err)
	}
	adminLog(v, r).Info("Updating tier %s", tier.Code)
	if err := s.userManager.UpdateTier(tier); err != nil {
		return mapUserErrorToHTTP(err)
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAdminTiersDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAdminTierDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	if _, err := s.userManager.Tier(req.Code); err != nil {
		return mapUserErrorToHTTP(err)
	}
	counts, err := s.userManager.TierUsersCount()
	if err != nil {
		return err
	} else if counts[req.Code] > 0 {
		return errHTTPConflictTierInUse
	}
	adminLog(v, r).Info("Removing tier %s", req.Code)
	if err := s.userManager.RemoveTier(req.Code); err != nil {
		return mapUserErrorToHTTP(err)
	}
	return s.writeJSON(w, newSuccessResponse())
}

// listenAdminSocket creates the socket's directory, removes a stale socket file (one with no
// server behind it) and listens. Anything at the path that is not a socket is left alone, in case
// listen-admin-unix points somewhere it should not (e.g. at the database). A socket that answers
// belongs to another running server; in that case this one does not take it over.
func (s *Server) listenAdminSocket() (net.Listener, error) {
	path := s.config.ListenAdminUnix
	if err := os.MkdirAll(filepath.Dir(path), adminSocketDirMode); err != nil {
		return nil, err
	}
	if reason := insecureAdminSocketDir(filepath.Dir(path)); reason != "" {
		// Not fatal: the path is the admin's choice
		log.Tag(tagStartup).Warn("Admin socket directory %s %s; other users may be able to use or replace the admin socket", filepath.Dir(path), reason)
	}
	if conn, err := net.DialTimeout("unix", path, adminSocketDialTimeout); err == nil {
		conn.Close()
		return nil, fmt.Errorf("another server is listening on %s", path)
	} else if errors.Is(err, fs.ErrPermission) {
		return nil, fmt.Errorf("%s belongs to a server of another user, set listen-admin-unix to a path of your own", path)
	}
	if stat, err := os.Lstat(path); err == nil {
		if stat.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket, refusing to replace it", path)
		} else if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, DefaultListenAdminUnixMode); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

// handleAdminSocket serves one admin API request that arrived over the local socket. There is
// no authentication: whoever can open the socket (file mode 0660, owner ntfy) is an admin.
// The request runs as adminSocketUser, bypasses rate limiting and the ban feed, and carries
// contextAdminSocket so handlers can allow what the network API refuses.
func (s *Server) handleAdminSocket(w http.ResponseWriter, r *http.Request) {
	r = withContext(r, map[contextKey]any{contextAdminSocket: true})
	v := newVisitor(s.visitors, adminSocketIP, adminSocketUser)
	adminLog(v, r).Debug("Admin socket request")
	if !slices.Contains(adminSocketPaths, r.URL.Path) {
		s.handleError(w, r, v, errHTTPNotFound) // Publishing, subscribing, the web app, etc. are not served here
		return
	}
	if err := s.handleInternal(w, r, v); err != nil {
		s.handleError(w, r, v, err)
	}
}

func (s *Server) killUserSubscriber(u *user.User, topicPattern string) error {
	topics, err := s.topicsFromPattern(topicPattern)
	if err != nil {
		return err
	}
	for _, t := range topics {
		t.CancelSubscriberUser(u.ID)
	}
	return nil
}

// broadcastAccessChanged tells the other cluster nodes that this node changed access entries, so
// they reload their access cache, which ends the streams that lost access there (see
// reauthorizeSubscribers). The user manager calls it after every access write. The signal is
// best effort: the periodic reload catches a lost one.
func (s *Server) broadcastAccessChanged() {
	s.cluster.BroadcastState(&cluster.State{
		AccessChanged: true,
	})
}

// handlePeerAccessChanged has the access cache reloaded after a peer changed access entries, never
// re-broadcasting it (loop prevention)
func (s *Server) handlePeerAccessChanged() {
	if s.userManager == nil {
		return
	}
	s.userManager.RequestAccessCacheReload() // Coalesced: a burst of peer changes costs at most two reloads
}

// userDeleted ends a deleted user's streams on this node and tells the other cluster nodes, which end
// theirs (see handleUsersDeleted). Called by both deletes: by an admin, and of one's own account.
func (s *Server) userDeleted(u *user.User) {
	s.handleUsersDeleted([]string{u.ID})
	s.cluster.BroadcastState(&cluster.State{
		DeletedUserIDs: []string{u.ID},
	})
}

// handleUsersDeleted ends all streams of the given (deleted) users on this node, on every topic,
// after a local delete or a peer's, never re-broadcasting. An access cache reload does not find
// them: a user without access entries (e.g. an admin) leaves nothing in the cache to compare.
func (s *Server) handleUsersDeleted(userIDs []string) {
	for _, t := range s.allTopics() {
		for _, userID := range userIDs {
			t.CancelSubscriberUser(userID) // An empty user ID cancels nobody
		}
	}
}

// reauthorizeSubscribers ends the subscriptions that changed access entries no longer allow, checking
// only matching topics. The user manager calls it after an access cache reload that found changes.
// It authorizes each subscription's own user as of subscribing, against the cache, without a
// database query; a deleted user's streams end through handleUsersDeleted instead.
func (s *Server) reauthorizeSubscribers(changes []*user.AccessChange) {
	for _, change := range changes {
		for _, t := range s.topicsForAccessChange(change.TopicPattern) {
			t.CancelSubscribersWhere(func(u *user.User) bool {
				if change.Username != user.Everyone && (u == nil || u.Name != change.Username) {
					return false // The entry does not apply to this subscriber
				}
				return s.userManager.Authorize(u, t.ID, user.PermissionRead) != nil
			})
		}
	}
}

// authorizeSubscribed authorizes a new stream once more, right after it subscribed. An access change
// whose re-check (see reauthorizeSubscribers) ran between the request's authorization and its
// subscribing would otherwise miss the stream for good. Each check sees the other: the re-check runs
// after the cache was swapped, so either it finds the subscription or this check finds the new
// entries. Only with the access cache, where it is a lookup in memory and re-checks happen at all.
func (s *Server) authorizeSubscribed(v *visitor, topics []*topic) error {
	if s.userManager == nil || !s.config.AuthAccessCacheEnabled {
		return nil
	}
	for _, t := range topics {
		if err := s.userManager.Authorize(v.User(), t.ID, user.PermissionRead); err != nil {
			return errHTTPForbidden
		}
	}
	return nil
}

// topicsForAccessChange returns the local topics a changed access entry's topic or pattern matches
func (s *Server) topicsForAccessChange(topicPattern string) []*topic {
	if !strings.Contains(topicPattern, "*") {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if t, ok := s.topics[topicPattern]; ok {
			return []*topic{t}
		}
		return nil
	}
	topics, err := s.topicsFromPattern(topicPattern)
	if err != nil {
		log.Tag(tagSubscribe).Err(err).Warn("Cannot match topics against access pattern %s", topicPattern)
		return nil
	}
	return topics
}

// checkRoleChange validates a requested role change. Roles can only be set over the admin socket.
func checkRoleChange(r *http.Request, role string) (user.Role, error) {
	if !isAdminSocketRequest(r) {
		return "", errHTTPForbiddenAdminSocketOnly
	} else if !user.AllowedRole(user.Role(role)) {
		return "", errHTTPBadRequestRoleInvalid
	}
	return user.Role(role), nil
}

// readAdminTierRequest reads and validates an apiAdminTier request, and converts it to a user.Tier
func readAdminTierRequest(r *http.Request) (*user.Tier, error) {
	req, err := readJSONWithLimit[apiAdminTier](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return nil, err
	} else if !user.AllowedTier(req.Code) || strings.TrimSpace(req.Name) == "" {
		return nil, errHTTPBadRequestTierCodeInvalid
	}
	return &user.Tier{
		Code:                     req.Code,
		Name:                     strings.TrimSpace(req.Name),
		MessageLimit:             req.MessagesLimit,
		MessageExpiryDuration:    time.Duration(req.MessagesExpiryDuration) * time.Second,
		EmailLimit:               req.EmailsLimit,
		CallLimit:                req.CallsLimit,
		ReservationLimit:         req.ReservationsLimit,
		AttachmentFileSizeLimit:  req.AttachmentFileSizeLimit,
		AttachmentTotalSizeLimit: req.AttachmentTotalSizeLimit,
		AttachmentExpiryDuration: time.Duration(req.AttachmentExpiryDuration) * time.Second,
		AttachmentBandwidthLimit: req.AttachmentBandwidthLimit,
		StripeMonthlyPriceID:     req.StripeMonthlyPriceID,
		StripeYearlyPriceID:      req.StripeYearlyPriceID,
	}, nil
}

func newAPIUserTokenResponse(username string, t *user.Token) *apiUserTokenResponse {
	response := &apiUserTokenResponse{
		Username:    username,
		Token:       t.Value,
		Label:       t.Label,
		Expires:     t.Expires.Unix(),
		Provisioned: t.Provisioned,
	}
	// Tokens created without a request (admin API, CLI, provisioning) carry their creation time and
	// origin 0.0.0.0 until first used; report those as never used
	if t.LastOrigin.IsValid() && !t.LastOrigin.IsUnspecified() {
		response.LastAccess = t.LastAccess.Unix()
		response.LastOrigin = t.LastOrigin.String()
	}
	return response
}

// mapUserErrorToHTTP maps user manager errors to HTTP errors
func mapUserErrorToHTTP(err error) error {
	switch {
	case errors.Is(err, user.ErrUserNotFound):
		return errHTTPBadRequestUserNotFound
	case errors.Is(err, user.ErrUserExists):
		return errHTTPConflictUserExists
	case errors.Is(err, user.ErrTierNotFound):
		return errHTTPBadRequestTierInvalid
	case errors.Is(err, user.ErrTokenNotFound):
		return errHTTPBadRequestTokenNotFound
	case errors.Is(err, user.ErrProvisionedUserChange):
		return errHTTPConflictProvisionedUserChange
	case errors.Is(err, user.ErrProvisionedTokenChange):
		return errHTTPConflictProvisionedTokenChange
	case errors.Is(err, user.ErrTooManyReservations):
		return errHTTPBadRequestTierReservationsLimit
	case errors.Is(err, user.ErrPasswordHashInvalid), errors.Is(err, user.ErrPasswordHashWeak):
		return errHTTPBadRequestPasswordHashInvalid
	case errors.Is(err, user.ErrInvalidArgument):
		return errHTTPBadRequest
	}
	return err
}

// isAdminSocketRequest reports whether the request came in over the local admin socket. Those
// requests may do what the network API refuses: change roles, and change or delete admins.
func isAdminSocketRequest(r *http.Request) bool {
	socket, err := fromContext[bool](r, contextAdminSocket)
	return err == nil && socket
}

// adminLog returns the audit log event for a mutating admin request, with how it arrived
func adminLog(v *visitor, r *http.Request) *log.Event {
	via := "http"
	if isAdminSocketRequest(r) {
		via = "socket"
	}
	return logvr(v, r).Tag(tagAdmin).Field("admin_via", via)
}
