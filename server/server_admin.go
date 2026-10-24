package server

import (
	"errors"
	"net/http"
	"strings"

	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"
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
		tier := ""
		if u.Tier != nil {
			tier = u.Tier.Code
		}
		userGrants := make([]*apiUserGrantResponse, len(grants[u.ID]))
		for i, g := range grants[u.ID] {
			userGrants[i] = &apiUserGrantResponse{
				Topic:      g.TopicPattern,
				Permission: g.Permission.String(),
			}
		}
		usersResponse[i] = &apiUserResponse{
			Username: u.Name,
			Role:     string(u.Role),
			Tier:     tier,
			Grants:   userGrants,
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
	u, err := s.userManager.User(req.Username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return err
	} else if u != nil {
		return errHTTPConflictUserExists
	}
	var tier *user.Tier
	if req.Tier != "" {
		tier, err = s.userManager.Tier(req.Tier)
		if errors.Is(err, user.ErrTierNotFound) {
			return errHTTPBadRequestTierInvalid
		} else if err != nil {
			return err
		}
	}
	password, hashed := req.Password, false
	if req.Hash != "" {
		password, hashed = req.Hash, true
	}
	if err := s.userManager.AddUser(req.Username, password, user.RoleUser, hashed); err != nil {
		return err
	}
	if tier != nil {
		if err := s.userManager.ChangeTier(req.Username, req.Tier); err != nil {
			return err
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
	} else if req.Password == "" && req.Hash == "" && req.Tier == "" {
		return errHTTPBadRequest.Wrap("need to provide at least one of \"password\", \"password_hash\" or \"tier\"")
	}
	u, err := s.userManager.User(req.Username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return err
	} else if u != nil {
		if u.IsAdmin() {
			return errHTTPForbidden
		}
		if req.Hash != "" {
			if err := s.userManager.ChangePassword(req.Username, req.Hash, true); errors.Is(err, user.ErrProvisionedUserChange) {
				return errHTTPConflictProvisionedUserChange
			} else if err != nil {
				return err
			}
		} else if req.Password != "" {
			if err := s.userManager.ChangePassword(req.Username, req.Password, false); errors.Is(err, user.ErrProvisionedUserChange) {
				return errHTTPConflictProvisionedUserChange
			} else if err != nil {
				return err
			}
		}
	} else {
		password, hashed := req.Password, false
		if req.Hash != "" {
			password, hashed = req.Hash, true
		}
		if err := s.userManager.AddUser(req.Username, password, user.RoleUser, hashed); err != nil {
			return err
		}
	}
	if req.Tier != "" {
		if _, err = s.userManager.Tier(req.Tier); errors.Is(err, user.ErrTierNotFound) {
			return errHTTPBadRequestTierInvalid
		} else if err != nil {
			return err
		}
		if err := s.userManager.ChangeTier(req.Username, req.Tier); err != nil {
			return err
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
	if errors.Is(err, user.ErrUserNotFound) {
		return errHTTPBadRequestUserNotFound
	} else if err != nil {
		return err
	} else if !u.IsUser() {
		return errHTTPUnauthorized.Wrap("can only remove regular users from API")
	}
	if err := s.userManager.RemoveUser(req.Username); errors.Is(err, user.ErrProvisionedUserChange) {
		return errHTTPConflictProvisionedUserChange
	} else if err != nil {
		return err
	}
	s.handleUsersDeleted([]string{u.ID}) // FIXME super inefficient
	s.cluster.BroadcastState(&cluster.State{
		DeletedUserIDs: []string{u.ID},
	})
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccessAllow(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccessAllowRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	_, err = s.userManager.User(req.Username)
	if errors.Is(err, user.ErrUserNotFound) {
		return errHTTPBadRequestUserNotFound
	} else if err != nil {
		return err
	}
	permission, err := user.ParsePermission(req.Permission)
	if err != nil {
		return errHTTPBadRequestPermissionInvalid
	}
	if err := s.userManager.AllowAccess(req.Username, req.Topic, permission); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccessReset(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccessResetRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if err != nil {
		return err
	}
	if err := s.userManager.ResetAccess(req.Username, req.Topic); err != nil {
		return err
	}
	if err := s.killUserSubscriber(u, req.Topic); err != nil { // This may be a pattern
		return err
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

// handleUsersDeleted ends all streams of the given (deleted) users on this node, on every topic,
// after a local delete or a peer's, never re-broadcasting. An access cache reload does not find
// them: a user without access entries (e.g. an admin) leaves nothing in the cache to compare.
func (s *Server) handleUsersDeleted(userIDs []string) {
	s.mu.RLock()
	topics := make([]*topic, 0, len(s.topics))
	for _, t := range s.topics {
		topics = append(topics, t)
	}
	s.mu.RUnlock()
	for _, t := range topics {
		for _, userID := range userIDs {
			t.CancelSubscriberUser(userID) // An empty user ID cancels nobody
		}
	}
}

// reauthorizeSubscribers ends the subscriptions that changed access entries no longer allow, checking
// only matching topics. The user manager calls it after an access cache reload that found changes.
// It authorizes each subscriber's user as of subscribing, against the cache, without a database
// query; a deleted user's streams end through handleUsersDeleted instead.
func (s *Server) reauthorizeSubscribers(changes []*user.AccessChange) {
	for _, change := range changes {
		for _, t := range s.topicsForAccessChange(change.Topic) {
			for userID, u := range t.SubscriberUsers() {
				if change.Username != user.Everyone && (u == nil || u.Name != change.Username) {
					continue // The entry does not apply to this subscriber
				} else if s.userManager.Authorize(u, t.ID, user.PermissionRead) == nil {
					continue // Still allowed
				}
				log.Tag(tagSubscribe).With(t).Debug("Canceling subscribers of user %s: access to the topic changed", userID)
				if userID == "" {
					t.CancelAnonymousSubscribers()
				} else {
					t.CancelSubscriberUser(userID)
				}
			}
		}
	}
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
