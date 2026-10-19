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
	if err := s.cancelSubscriberUserLocal(u.ID, topicPattern); err != nil {
		return err
	}
	// The topic/subscriber registry is per-node; ask peer nodes to kick the user too
	s.cluster.BroadcastState(&cluster.State{SubscriberCancels: []*cluster.SubscriberCancel{{Topic: topicPattern, UserID: u.ID}}})
	return nil
}

func (s *Server) cancelSubscriberUserLocal(userID, topicPattern string) error {
	topics, err := s.topicsFromPattern(topicPattern)
	if err != nil {
		return err
	}
	for _, t := range topics {
		t.CancelSubscriberUser(userID)
	}
	return nil
}

// applySubscriberCancel applies a peer node's subscriber-cancel request to local connections
// only; it never re-broadcasts (loop prevention, see cluster.CancelFunc).
//
// It first refreshes this node's access cache for everyone and the user concerned: without it a
// canceled subscriber could reconnect at once and be let in by the stale cache (its periodic
// reload is minutes away). The refresh also re-checks the affected subscribers (see
// reauthorizeSubscribers); the explicit cancel below covers nodes without the cache.
func (s *Server) applySubscriberCancel(cancel *cluster.SubscriberCancel) {
	if s.userManager != nil {
		usernames := []string{user.Everyone}
		for _, userID := range []string{cancel.UserID, cancel.ExceptUserID} {
			if userID == "" {
				continue
			} else if u, err := s.userManager.UserByID(userID); err == nil {
				usernames = append(usernames, u.Name)
			} else if !errors.Is(err, user.ErrUserNotFound) {
				log.Tag(tagSubscribe).Err(err).Warn("Cannot look up user %s for a peer's subscriber cancel", userID)
			}
		}
		if err := s.userManager.ReloadAccessCache(usernames...); err != nil {
			log.Tag(tagSubscribe).Err(err).Warn("Cannot reload the access cache for a peer's subscriber cancel")
		}
	}
	if cancel.ExceptUserID != "" {
		s.mu.RLock()
		t, ok := s.topics[cancel.Topic]
		s.mu.RUnlock()
		if ok {
			t.CancelSubscribersExceptUser(cancel.ExceptUserID)
		}
		return
	}
	if err := s.cancelSubscriberUserLocal(cancel.UserID, cancel.Topic); err != nil {
		log.Tag(tagSubscribe).Err(err).Warn("Cannot apply peer subscriber cancel for topic %s", cancel.Topic)
	}
}

// reauthorizeSubscribers ends the live subscriptions that changed access entries no longer let
// read their topic. The user manager calls it when an access cache reload finds entries
// changed: by this node, by a peer (relayed or not), or by the ntfy access CLI. Only topics
// matching a changed entry are checked, and on them only the subscribers the entry applies to
// (an entry for everyone applies to all). A user that cannot be looked up keeps the
// subscription, so that a database hiccup does not disconnect anyone; a deleted one does not.
func (s *Server) reauthorizeSubscribers(changes []*user.AccessChange) {
	users := make(map[string]*user.User) // Resolved once per call; nil means anonymous or deleted
	for _, change := range changes {
		for _, t := range s.topicsForAccessChange(change.Topic) {
			for _, userID := range t.SubscriberUserIDs() {
				u, resolved := users[userID]
				if !resolved && userID != "" {
					var err error
					if u, err = s.userManager.UserByID(userID); err != nil && !errors.Is(err, user.ErrUserNotFound) {
						log.Tag(tagSubscribe).Err(err).Warn("Cannot re-check the subscribers of user %s", userID)
						continue
					}
					users[userID] = u
				}
				deleted := userID != "" && u == nil
				if !deleted && change.Username != user.Everyone && (u == nil || u.Name != change.Username) {
					continue // The entry does not apply to this subscriber
				} else if deleted || s.userManager.Authorize(u, t.ID, user.PermissionRead) != nil {
					log.Tag(tagSubscribe).With(t).Debug("Canceling subscribers of user %s: access to the topic changed", userID)
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
