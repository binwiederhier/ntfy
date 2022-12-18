package server

import (
	"encoding/json"
	"errors"
	"heckel.io/ntfy/auth"
	"heckel.io/ntfy/util"
	"net/http"
)

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	signupAllowed := s.config.EnableSignup
	admin := v.user != nil && v.user.Role == auth.RoleAdmin
	if !signupAllowed && !admin {
		return errHTTPUnauthorized
	}
	body, err := util.Peek(r.Body, 4096) // FIXME
	if err != nil {
		return err
	}
	defer r.Body.Close()
	var newAccount apiAccountCreateRequest
	if err := json.NewDecoder(body).Decode(&newAccount); err != nil {
		return err
	}
	if err := s.auth.AddUser(newAccount.Username, newAccount.Password, auth.RoleUser); err != nil { // TODO this should return a User
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	// FIXME return something
	return nil
}

func (s *Server) handleAccountGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	stats, err := v.Stats()
	if err != nil {
		return err
	}
	response := &apiAccountSettingsResponse{
		Usage: &apiAccountUsageLimits{},
	}
	if v.user != nil {
		response.Username = v.user.Name
		response.Role = string(v.user.Role)
		if v.user.Prefs != nil {
			if v.user.Prefs.Language != "" {
				response.Language = v.user.Prefs.Language
			}
			if v.user.Prefs.Notification != nil {
				response.Notification = v.user.Prefs.Notification
			}
			if v.user.Prefs.Subscriptions != nil {
				response.Subscriptions = v.user.Prefs.Subscriptions
			}
		}
		if v.user.Plan != nil {
			response.Usage.Basis = "account"
			response.Plan = &apiAccountSettingsPlan{
				Name:                  v.user.Plan.Name,
				MessagesLimit:         v.user.Plan.MessagesLimit,
				EmailsLimit:           v.user.Plan.EmailsLimit,
				AttachmentsBytesLimit: v.user.Plan.AttachmentBytesLimit,
			}
		} else {
			if v.user.Role == auth.RoleAdmin {
				response.Usage.Basis = "account"
				response.Plan = &apiAccountSettingsPlan{
					Name:                  "Unlimited",
					MessagesLimit:         0,
					EmailsLimit:           0,
					AttachmentsBytesLimit: 0,
				}
			} else {
				response.Usage.Basis = "ip"
				response.Plan = &apiAccountSettingsPlan{
					Name:                  "Free",
					MessagesLimit:         s.config.VisitorRequestLimitBurst,
					EmailsLimit:           s.config.VisitorEmailLimitBurst,
					AttachmentsBytesLimit: s.config.VisitorAttachmentTotalSizeLimit,
				}
			}
		}
	} else {
		response.Username = auth.Everyone
		response.Role = string(auth.RoleAnonymous)
		response.Usage.Basis = "account"
		response.Plan = &apiAccountSettingsPlan{
			Name:                  "Anonymous",
			MessagesLimit:         s.config.VisitorRequestLimitBurst,
			EmailsLimit:           s.config.VisitorEmailLimitBurst,
			AttachmentsBytesLimit: s.config.VisitorAttachmentTotalSizeLimit,
		}
	}
	response.Usage.Messages = int(v.requests.Tokens())
	response.Usage.AttachmentsBytes = stats.VisitorAttachmentBytesUsed
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user == nil {
		return errHTTPUnauthorized
	}
	if err := s.auth.RemoveUser(v.user.Name); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	// FIXME return something
	return nil
}

func (s *Server) handleAccountPasswordChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user == nil {
		return errHTTPUnauthorized
	}
	body, err := util.Peek(r.Body, 4096) // FIXME
	if err != nil {
		return err
	}
	defer r.Body.Close()
	var newPassword apiAccountCreateRequest // Re-use!
	if err := json.NewDecoder(body).Decode(&newPassword); err != nil {
		return err
	}
	if err := s.auth.ChangePassword(v.user.Name, newPassword.Password); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	// FIXME return something
	return nil
}

func (s *Server) handleAccountTokenGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// TODO rate limit
	if v.user == nil {
		return errHTTPUnauthorized
	}
	token, err := s.auth.CreateToken(v.user)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	response := &apiAccountTokenResponse{
		Token: token,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountTokenDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// TODO rate limit
	if v.user == nil || v.user.Token == "" {
		return errHTTPUnauthorized
	}
	if err := s.auth.RemoveToken(v.user); err != nil {
		return err
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}

func (s *Server) handleAccountSettingsChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user == nil {
		return errors.New("no user")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	body, err := util.Peek(r.Body, 4096)               // FIXME
	if err != nil {
		return err
	}
	defer r.Body.Close()
	var newPrefs auth.UserPrefs
	if err := json.NewDecoder(body).Decode(&newPrefs); err != nil {
		return err
	}
	if v.user.Prefs == nil {
		v.user.Prefs = &auth.UserPrefs{}
	}
	prefs := v.user.Prefs
	if newPrefs.Language != "" {
		prefs.Language = newPrefs.Language
	}
	if newPrefs.Notification != nil {
		if prefs.Notification == nil {
			prefs.Notification = &auth.UserNotificationPrefs{}
		}
		if newPrefs.Notification.DeleteAfter > 0 {
			prefs.Notification.DeleteAfter = newPrefs.Notification.DeleteAfter
		}
		if newPrefs.Notification.Sound != "" {
			prefs.Notification.Sound = newPrefs.Notification.Sound
		}
		if newPrefs.Notification.MinPriority > 0 {
			prefs.Notification.MinPriority = newPrefs.Notification.MinPriority
		}
	}
	return s.auth.ChangeSettings(v.user)
}

func (s *Server) handleAccountSubscriptionAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user == nil {
		return errors.New("no user")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	body, err := util.Peek(r.Body, 4096)               // FIXME
	if err != nil {
		return err
	}
	defer r.Body.Close()
	var newSubscription auth.UserSubscription
	if err := json.NewDecoder(body).Decode(&newSubscription); err != nil {
		return err
	}
	if v.user.Prefs == nil {
		v.user.Prefs = &auth.UserPrefs{}
	}
	newSubscription.ID = "" // Client cannot set ID
	for _, subscription := range v.user.Prefs.Subscriptions {
		if newSubscription.BaseURL == subscription.BaseURL && newSubscription.Topic == subscription.Topic {
			newSubscription = *subscription
			break
		}
	}
	if newSubscription.ID == "" {
		newSubscription.ID = util.RandomString(16)
		v.user.Prefs.Subscriptions = append(v.user.Prefs.Subscriptions, &newSubscription)
		if err := s.auth.ChangeSettings(v.user); err != nil {
			return err
		}
	}
	if err := json.NewEncoder(w).Encode(newSubscription); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user == nil {
		return errors.New("no user")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	matches := accountSubscriptionSingleRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidFilePath // FIXME
	}
	subscriptionID := matches[1]
	if v.user.Prefs == nil || v.user.Prefs.Subscriptions == nil {
		return nil
	}
	newSubscriptions := make([]*auth.UserSubscription, 0)
	for _, subscription := range v.user.Prefs.Subscriptions {
		if subscription.ID != subscriptionID {
			newSubscriptions = append(newSubscriptions, subscription)
		}
	}
	if len(newSubscriptions) < len(v.user.Prefs.Subscriptions) {
		v.user.Prefs.Subscriptions = newSubscriptions
		if err := s.auth.ChangeSettings(v.user); err != nil {
			return err
		}
	}
	return nil
}
