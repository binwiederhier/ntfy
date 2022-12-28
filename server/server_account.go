package server

import (
	"encoding/json"
	"errors"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"io"
	"net/http"
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
	body, err := util.Peek(r.Body, 4096) // FIXME
	if err != nil {
		return err
	}
	defer r.Body.Close()
	var newAccount apiAccountCreateRequest
	if err := json.NewDecoder(body).Decode(&newAccount); err != nil {
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
	// FIXME return something
	return nil
}

func (s *Server) handleAccountGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
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
			AttachmentTotalSize:          stats.AttachmentTotalSize,
			AttachmentTotalSizeRemaining: stats.AttachmentTotalSizeRemaining,
		},
		Limits: &apiAccountLimits{
			Basis:               stats.Basis,
			Messages:            stats.MessagesLimit,
			Emails:              stats.EmailsLimit,
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

	} else {
		response.Username = user.Everyone
		response.Role = string(user.RoleAnonymous)
		response.Plan = &apiAccountPlan{
			Code:       string(user.PlanNone),
			Upgradable: true,
		}
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user == nil {
		return errHTTPUnauthorized
	}
	if err := s.userManager.RemoveUser(v.user.Name); err != nil {
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
	if err := s.userManager.ChangePassword(v.user.Name, newPassword.Password); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	// FIXME return something
	return nil
}

func (s *Server) handleAccountTokenIssue(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// TODO rate limit
	if v.user == nil {
		return errHTTPUnauthorized
	}
	token, err := s.userManager.CreateToken(v.user)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	response := &apiAccountTokenResponse{
		Token:   token.Value,
		Expires: token.Expires,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountTokenExtend(w http.ResponseWriter, r *http.Request, v *visitor) error {
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
		Expires: token.Expires,
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
	if err := s.userManager.RemoveToken(v.user); err != nil {
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
	var newPrefs user.Prefs
	if err := json.NewDecoder(body).Decode(&newPrefs); err != nil {
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
	return s.userManager.ChangeSettings(v.user)
}

func (s *Server) handleAccountSubscriptionAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user == nil {
		return errors.New("no user")
	}
	newSubscription, err := readJSONBody[user.Subscription](r.Body)
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
		newSubscription.ID = util.RandomString(16)
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
	if v.user == nil {
		return errors.New("no user") // FIXME s.ensureUser
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	matches := accountSubscriptionSingleRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidFilePath // FIXME
	}
	updatedSubscription, err := readJSONBody[user.Subscription](r.Body)
	if err != nil {
		return err
	}
	subscriptionID := matches[1]
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
	return nil
}

func readJSONBody[T any](body io.ReadCloser) (*T, error) {
	body, err := util.Peek(body, 4096)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var obj T
	if err := json.NewDecoder(body).Decode(&obj); err != nil {
		return nil, err
	}
	return &obj, nil
}
