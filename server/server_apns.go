package server

import (
	"net/http"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

type apiAPNSRegisterRequest struct {
	Token string `json:"token"`
	Topic string `json:"topic"`
}

func (s *Server) handleAPNSRegister(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if s.apnsStore == nil || s.apnsClient == nil {
		return errHTTPBadRequestAPNSNotEnabled
	}
	req, err := readJSONWithLimit[apiAPNSRegisterRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil || req.Token == "" || req.Topic == "" {
		return errHTTPBadRequestAPNSRegistrationInvalid
	}
	// Check user permissions to access/read this topic
	if s.userManager != nil {
		u := v.User()
		if err := s.userManager.Authorize(u, req.Topic, user.PermissionRead); err != nil {
			logvr(v, r).Err(err).Debug("Access to topic %s not authorized", req.Topic)
			return errHTTPForbidden
		}
	}
	if err := s.apnsStore.Register(req.Token, req.Topic, v.MaybeUserID(), v.IP()); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAPNSUnregister(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	if s.apnsStore == nil || s.apnsClient == nil {
		return errHTTPBadRequestAPNSNotEnabled
	}
	req, err := readJSONWithLimit[apiAPNSRegisterRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil || req.Token == "" || req.Topic == "" {
		return errHTTPBadRequestAPNSRegistrationInvalid
	}
	if err := s.apnsStore.Unregister(req.Token, req.Topic); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) publishToAPNSEndpoints(v *visitor, m *model.Message) {
	if s.apnsStore == nil || s.apnsClient == nil {
		return
	}
	tokens, err := s.apnsStore.GetTokens(m.Topic)
	if err != nil {
		logvm(v, m).Err(err).Warn("Unable to query APNs registered tokens for topic %s", m.Topic)
		return
	}
	if len(tokens) == 0 {
		return
	}
	logvm(v, m).Debug("Publishing direct APNs message to %d subscribers", len(tokens))
	var auther user.Auther
	if s.userManager != nil {
		auther = s.userManager
	}
	for _, token := range tokens {
		go func(tok string) {
			if err := s.apnsClient.Send(tok, m, auther); err != nil {
				logvm(v, m).Err(err).Warn("Failed to send direct APNs notification to token %s", tok)
			} else {
				logvm(v, m).Debug("Direct APNs notification delivered to token %s", tok)
			}
		}(token)
	}
}
