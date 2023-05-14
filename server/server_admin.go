package server

import (
	"heckel.io/ntfy/user"
	"net/http"
)

func (s *Server) handleUserAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUserAddRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !user.AllowedUsername(req.Username) || req.Password == "" {
		return errHTTPBadRequest.Wrap("username invalid, or password missing")
	}
	u, err := s.userManager.User(req.Username)
	if err != nil && err != user.ErrUserNotFound {
		return err
	} else if u != nil {
		return errHTTPConflictUserExists
	}
	var tier *user.Tier
	if req.Tier != "" {
		tier, err = s.userManager.Tier(req.Tier)
		if err == user.ErrTierNotFound {
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

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUserDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if err == user.ErrUserNotFound {
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
