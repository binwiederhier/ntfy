package server

import (
	"errors"
	"heckel.io/ntfy/v2/user"
	"net/http"
)

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
				Permission: g.Allow.String(),
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
	} else if !user.AllowedUsername(req.Username) || req.Password == "" {
		return errHTTPBadRequest.Wrap("username invalid, or password missing")
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
	if err := s.userManager.AddUser(req.Username, req.Password, user.RoleUser); err != nil {
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
	} else if !user.AllowedUsername(req.Username) || req.Password == "" {
		return errHTTPBadRequest.Wrap("username invalid, or password missing")
	}
	u, err := s.userManager.User(req.Username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return err
	} else if u != nil {
		if u.IsAdmin() {
			return errHTTPForbidden
		}
		if err := s.userManager.ChangePassword(req.Username, req.Password); err != nil {
			return err
		}
		return s.writeJSON(w, newSuccessResponse())
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
	if err := s.userManager.AddUser(req.Username, req.Password, user.RoleUser); err != nil {
		return err
	}
	if tier != nil {
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
	if err := s.userManager.RemoveUser(req.Username); err != nil {
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
	topics, err := s.topicsFromPattern(topicPattern)
	if err != nil {
		return err
	}
	for _, t := range topics {
		t.CancelSubscriberUser(u.ID)
	}
	return nil
}
