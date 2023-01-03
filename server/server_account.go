package server

import (
	"encoding/json"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"net/http"
)

const (
	jsonBodyBytesLimit   = 4096
	subscriptionIDLength = 16
)

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	admin := v.user != nil && v.user.Role == user.RoleAdmin
	if !admin {
		if !s.config.EnableSignup {
			return errHTTPBadRequestSignupNotEnabled
		} else if v.user != nil {
			return errHTTPUnauthorized // Cannot create account from user context
		}
	}
	newAccount, err := readJSONWithLimit[apiAccountCreateRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	if existingUser, _ := s.userManager.User(newAccount.Username); existingUser != nil {
		return errHTTPConflictUserExists
	}
	if v.accountLimiter != nil && !v.accountLimiter.Allow() {
		return errHTTPTooManyRequestsAccountCreateLimit
	}
	if err := s.userManager.AddUser(newAccount.Username, newAccount.Password, user.RoleUser); err != nil { // TODO this should return a User
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}

func (s *Server) handleAccountGet(w http.ResponseWriter, _ *http.Request, v *visitor) error {
	stats, err := v.Info()
	if err != nil {
		return err
	}
	response := &apiAccountResponse{
		Stats: &apiAccountStats{
			Messages:                     stats.Messages,
			MessagesRemaining:            stats.MessagesRemaining,
			Emails:                       stats.Emails,
			EmailsRemaining:              stats.EmailsRemaining,
			Topics:                       stats.Topics,
			TopicsRemaining:              stats.TopicsRemaining,
			AttachmentTotalSize:          stats.AttachmentTotalSize,
			AttachmentTotalSizeRemaining: stats.AttachmentTotalSizeRemaining,
		},
		Limits: &apiAccountLimits{
			Basis:               stats.Basis,
			Messages:            stats.MessagesLimit,
			Emails:              stats.EmailsLimit,
			Topics:              stats.TopicsLimit,
			AttachmentTotalSize: stats.AttachmentTotalSizeLimit,
			AttachmentFileSize:  stats.AttachmentFileSizeLimit,
		},
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
			response.Plan = &apiAccountPlan{
				Code:       v.user.Plan.Code,
				Upgradable: v.user.Plan.Upgradable,
			}
		} else if v.user.Role == user.RoleAdmin {
			response.Plan = &apiAccountPlan{
				Code:       string(user.PlanUnlimited),
				Upgradable: false,
			}
		} else {
			response.Plan = &apiAccountPlan{
				Code:       string(user.PlanDefault),
				Upgradable: true,
			}
		}
		reservations, err := s.userManager.Reservations(v.user.Name)
		if err != nil {
			return err
		}
		if len(reservations) > 0 {
			response.Reservations = make([]*apiAccountReservation, 0)
			for _, r := range reservations {
				response.Reservations = append(response.Reservations, &apiAccountReservation{
					Topic:    r.Topic,
					Everyone: r.Everyone.String(),
				})
			}
		}
	} else {
		response.Username = user.Everyone
		response.Role = string(user.RoleAnonymous)
		response.Plan = &apiAccountPlan{
			Code:       string(user.PlanNone),
			Upgradable: true,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountDelete(w http.ResponseWriter, _ *http.Request, v *visitor) error {
	if err := s.userManager.RemoveUser(v.user.Name); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}

func (s *Server) handleAccountPasswordChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	newPassword, err := readJSONWithLimit[apiAccountPasswordChangeRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	if err := s.userManager.ChangePassword(v.user.Name, newPassword.Password); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}

func (s *Server) handleAccountTokenIssue(w http.ResponseWriter, _ *http.Request, v *visitor) error {
	// TODO rate limit
	token, err := s.userManager.CreateToken(v.user)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	response := &apiAccountTokenResponse{
		Token:   token.Value,
		Expires: token.Expires.Unix(),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountTokenExtend(w http.ResponseWriter, _ *http.Request, v *visitor) error {
	// TODO rate limit
	if v.user == nil {
		return errHTTPUnauthorized
	} else if v.user.Token == "" {
		return errHTTPBadRequestNoTokenProvided
	}
	token, err := s.userManager.ExtendToken(v.user)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	response := &apiAccountTokenResponse{
		Token:   token.Value,
		Expires: token.Expires.Unix(),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountTokenDelete(w http.ResponseWriter, _ *http.Request, v *visitor) error {
	// TODO rate limit
	if v.user.Token == "" {
		return errHTTPBadRequestNoTokenProvided
	}
	if err := s.userManager.RemoveToken(v.user); err != nil {
		return err
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}

func (s *Server) handleAccountSettingsChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	newPrefs, err := readJSONWithLimit[user.Prefs](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	if v.user.Prefs == nil {
		v.user.Prefs = &user.Prefs{}
	}
	prefs := v.user.Prefs
	if newPrefs.Language != "" {
		prefs.Language = newPrefs.Language
	}
	if newPrefs.Notification != nil {
		if prefs.Notification == nil {
			prefs.Notification = &user.NotificationPrefs{}
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
	if err := s.userManager.ChangeSettings(v.user); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}

func (s *Server) handleAccountSubscriptionAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	newSubscription, err := readJSONWithLimit[user.Subscription](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	if v.user.Prefs == nil {
		v.user.Prefs = &user.Prefs{}
	}
	newSubscription.ID = "" // Client cannot set ID
	for _, subscription := range v.user.Prefs.Subscriptions {
		if newSubscription.BaseURL == subscription.BaseURL && newSubscription.Topic == subscription.Topic {
			newSubscription = subscription
			break
		}
	}
	if newSubscription.ID == "" {
		newSubscription.ID = util.RandomString(subscriptionIDLength)
		v.user.Prefs.Subscriptions = append(v.user.Prefs.Subscriptions, newSubscription)
		if err := s.userManager.ChangeSettings(v.user); err != nil {
			return err
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	if err := json.NewEncoder(w).Encode(newSubscription); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountSubscriptionChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := accountSubscriptionSingleRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	subscriptionID := matches[1]
	updatedSubscription, err := readJSONWithLimit[user.Subscription](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	if v.user.Prefs == nil || v.user.Prefs.Subscriptions == nil {
		return errHTTPNotFound
	}
	var subscription *user.Subscription
	for _, sub := range v.user.Prefs.Subscriptions {
		if sub.ID == subscriptionID {
			sub.DisplayName = updatedSubscription.DisplayName
			subscription = sub
			break
		}
	}
	if subscription == nil {
		return errHTTPNotFound
	}
	if err := s.userManager.ChangeSettings(v.user); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	if err := json.NewEncoder(w).Encode(subscription); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := accountSubscriptionSingleRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	subscriptionID := matches[1]
	if v.user.Prefs == nil || v.user.Prefs.Subscriptions == nil {
		return nil
	}
	newSubscriptions := make([]*user.Subscription, 0)
	for _, subscription := range v.user.Prefs.Subscriptions {
		if subscription.ID != subscriptionID {
			newSubscriptions = append(newSubscriptions, subscription)
		}
	}
	if len(newSubscriptions) < len(v.user.Prefs.Subscriptions) {
		v.user.Prefs.Subscriptions = newSubscriptions
		if err := s.userManager.ChangeSettings(v.user); err != nil {
			return err
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}

func (s *Server) handleAccountAccessAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountAccessRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	if !topicRegex.MatchString(req.Topic) {
		return errHTTPBadRequestTopicInvalid
	}
	if err := s.userManager.CheckAllowAccess(v.user.Name, req.Topic); err != nil {
		return errHTTPConflictTopicReserved
	}
	owner, username := v.user.Name, v.user.Name
	everyone, err := user.ParsePermission(req.Everyone)
	if err != nil {
		return errHTTPBadRequestPermissionInvalid
	}
	if err := s.userManager.AllowAccess(owner, username, req.Topic, true, true); err != nil {
		return err
	}
	if err := s.userManager.AllowAccess(owner, user.Everyone, req.Topic, everyone.IsRead(), everyone.IsWrite()); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}

func (s *Server) handleAccountAccessDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := accountAccessSingleRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	topic := matches[1]
	if !topicRegex.MatchString(topic) {
		return errHTTPBadRequestTopicInvalid
	}
	reservations, err := s.userManager.Reservations(v.user.Name) // FIXME replace with HasReservation
	if err != nil {
		return err
	}
	authorized := false
	for _, r := range reservations {
		if r.Topic == topic {
			authorized = true
			break
		}
	}
	if !authorized {
		return errHTTPUnauthorized
	}
	if err := s.userManager.ResetAccess(v.user.Name, topic); err != nil {
		return err
	}
	if err := s.userManager.ResetAccess(user.Everyone, topic); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
}
