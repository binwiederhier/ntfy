package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"
)

const (
	apiTierReset = "-" // Tier value that removes a user's tier (PUT /v1/users); same marker the CLI uses
)

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
		return errHTTPBadRequest.Wrap("username invalid, or password/password_hash missing")
	}
	role := user.RoleUser
	if req.Role != "" {
		if role, err = readAdminRole(r, req.Role); err != nil {
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
			return mapAdminUserError(err)
		}
	}
	password, hashed := req.Password, false
	if req.Hash != "" {
		password, hashed = req.Hash, true
	}
	adminLog(v, r).Info("Adding user %s (role %s)", req.Username, role)
	if err := s.userManager.AddUser(req.Username, password, role, hashed); err != nil {
		return mapAdminUserError(err)
	}
	if req.Tier != "" {
		if err := s.userManager.ChangeTier(req.Username, req.Tier); err != nil {
			return mapAdminUserError(err)
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
		return errHTTPBadRequest.Wrap("need to provide at least one of \"password\", \"password_hash\", \"tier\" or \"role\"")
	}
	var role user.Role
	if req.Role != "" {
		if role, err = readAdminRole(r, req.Role); err != nil {
			return err
		}
	}
	if req.Tier != "" && req.Tier != apiTierReset {
		if _, err := s.userManager.Tier(req.Tier); err != nil {
			return mapAdminUserError(err)
		}
	}
	u, err := s.userManager.User(req.Username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return err
	} else if u != nil {
		if u.IsAdmin() && !isAdminSocketRequest(r) {
			return errHTTPForbidden
		}
		if req.Hash != "" || req.Password != "" {
			adminLog(v, r).Info("Changing password of user %s", req.Username)
			if req.Hash != "" {
				err = s.userManager.ChangePassword(req.Username, req.Hash, true)
			} else {
				err = s.userManager.ChangePassword(req.Username, req.Password, false)
			}
			if err != nil {
				return mapAdminUserError(err)
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
			return mapAdminUserError(err)
		}
	}
	if req.Tier == apiTierReset {
		adminLog(v, r).Info("Removing tier of user %s", req.Username)
		if err := s.userManager.ResetTier(req.Username); err != nil {
			return mapAdminUserError(err)
		}
	} else if req.Tier != "" {
		adminLog(v, r).Info("Changing tier of user %s to %s", req.Username, req.Tier)
		if err := s.userManager.ChangeTier(req.Username, req.Tier); err != nil {
			return mapAdminUserError(err)
		}
	}
	if req.Role != "" {
		adminLog(v, r).Info("Changing role of user %s to %s", req.Username, role)
		if err := s.userManager.ChangeRole(req.Username, role); err != nil {
			return mapAdminUserError(err)
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
		return mapAdminUserError(err)
	} else if !u.IsUser() && !isAdminSocketRequest(r) {
		return errHTTPUnauthorized.Wrap("can only remove regular users from API")
	} else if u.Name == user.Everyone {
		return errHTTPBadRequest.Wrap("cannot remove the everyone user")
	}
	adminLog(v, r).Info("Removing user %s", req.Username)
	if err := s.userManager.RemoveUser(req.Username); err != nil {
		return mapAdminUserError(err)
	}
	if s.webPush != nil {
		if err := s.webPush.RemoveSubscriptionsByUserID(u.ID); err != nil {
			adminLog(v, r).Err(err).Warn("Failed to remove web push subscriptions of user %s", req.Username)
		}
	}
	if err := s.killUserSubscriber(u, "*"); err != nil { // FIXME super inefficient
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccessAllow(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccessAllowRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if err != nil {
		return mapAdminUserError(err)
	} else if u.IsAdmin() {
		return errHTTPBadRequestAccessAdminUser
	}
	permission, err := user.ParsePermission(req.Permission)
	if err != nil {
		return errHTTPBadRequestPermissionInvalid
	}
	adminLog(v, r).Info("Granting %s access to topic %s for user %s", permission, req.Topic, req.Username)
	if err := s.userManager.AllowAccess(req.Username, req.Topic, permission); err != nil {
		return mapAdminUserError(err)
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccessReset(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccessResetRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if req.Username == "" && req.Topic != "" {
		return errHTTPBadRequest.Wrap("topic requires a username")
	}
	var u *user.User
	if req.Username != "" {
		if u, err = s.userManager.User(req.Username); err != nil {
			return mapAdminUserError(err)
		}
	}
	adminLog(v, r).Info("Resetting access for user %s, topic %s", req.Username, req.Topic)
	if err := s.userManager.ResetAccess(req.Username, req.Topic); err != nil {
		return mapAdminUserError(err)
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
			return mapAdminUserError(err)
		}
		users = []*user.User{u}
	} else {
		var err error
		if users, err = s.userManager.Users(); err != nil {
			return err
		}
	}
	response := make([]*apiUserTokenResponse, 0)
	for _, u := range users {
		if u.IsAdmin() && !isAdminSocketRequest(r) {
			continue // Tokens are secrets; the network API never touches admins
		}
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
	u, err := s.readAdminTokenUser(r, req.Username)
	if err != nil {
		return err
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
		return mapAdminUserError(err)
	}
	return s.writeJSON(w, newAPIUserTokenResponse(u.Name, token))
}

func (s *Server) handleUsersTokensDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUsersTokensDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.readAdminTokenUser(r, req.Username)
	if err != nil {
		return err
	}
	adminLog(v, r).Info("Removing token for user %s", req.Username)
	if err := s.userManager.RemoveToken(u.ID, req.Token); err != nil {
		return mapAdminUserError(err)
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
		return mapAdminUserError(err)
	} else if u.Provisioned {
		return errHTTPConflictProvisionedUserChange
	} else if u.IsAdmin() && !isAdminSocketRequest(r) {
		return errHTTPForbidden
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
		return mapAdminUserError(err)
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAdminTiersUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	tier, err := readAdminTierRequest(r)
	if err != nil {
		return err
	}
	if _, err := s.userManager.Tier(tier.Code); err != nil {
		return mapAdminUserError(err)
	}
	adminLog(v, r).Info("Updating tier %s", tier.Code)
	if err := s.userManager.UpdateTier(tier); err != nil {
		return mapAdminUserError(err)
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAdminTiersDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAdminTierDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	if _, err := s.userManager.Tier(req.Code); err != nil {
		return mapAdminUserError(err)
	}
	counts, err := s.userManager.TierUsersCount()
	if err != nil {
		return err
	} else if counts[req.Code] > 0 {
		return errHTTPConflictTierInUse
	}
	adminLog(v, r).Info("Removing tier %s", req.Code)
	if err := s.userManager.RemoveTier(req.Code); err != nil {
		return mapAdminUserError(err)
	}
	return s.writeJSON(w, newSuccessResponse())
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

// readAdminTokenUser looks up the owner of tokens to be created or removed. Admin users' tokens
// can only be managed over the admin socket.
func (s *Server) readAdminTokenUser(r *http.Request, username string) (*user.User, error) {
	u, err := s.userManager.User(username)
	if err != nil {
		return nil, mapAdminUserError(err)
	} else if u.IsAdmin() && !isAdminSocketRequest(r) {
		return nil, errHTTPForbidden
	}
	return u, nil
}

// readAdminRole validates a role in an admin request. Roles can only be set over the admin socket.
func readAdminRole(r *http.Request, role string) (user.Role, error) {
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
	return &apiUserTokenResponse{
		Username:    username,
		Token:       t.Value,
		Label:       t.Label,
		Expires:     t.Expires.Unix(),
		LastAccess:  t.LastAccess.Unix(),
		LastOrigin:  t.LastOrigin.String(),
		Provisioned: t.Provisioned,
	}
}

// mapAdminUserError maps user manager errors of admin operations to HTTP errors
func mapAdminUserError(err error) error {
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
	if !isAdminSocketRequest(r) {
		return logvr(v, r).Tag(tagAdmin).Field("admin_via", "http")
	}
	ev := logvr(v, r).Tag(tagAdmin).Field("admin_via", "socket")
	if peer, err := fromContext[peerCredentials](r, contextAdminSocketPeer); err == nil {
		ev = ev.Fields(log.Context{
			"peer_uid": peer.UID,
			"peer_gid": peer.GID,
			"peer_pid": peer.PID,
		})
	}
	return ev
}
