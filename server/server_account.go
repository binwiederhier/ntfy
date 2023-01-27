package server

import (
	"encoding/json"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"net/http"
	"strings"
)

const (
	subscriptionIDLength      = 16
	subscriptionIDPrefix      = "su_"
	syncTopicAccountSyncEvent = "sync"
)

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	admin := v.user != nil && v.user.Role == user.RoleAdmin
	if !admin {
		if !s.config.EnableSignup {
			return errHTTPBadRequestSignupNotEnabled
		} else if v.user != nil {
			return errHTTPUnauthorized // Cannot create account from user context
		}
		if !v.AccountCreationAllowed() {
			return errHTTPTooManyRequestsLimitAccountCreation
		}
	}
	newAccount, err := readJSONWithLimit[apiAccountCreateRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	if existingUser, _ := s.userManager.User(newAccount.Username); existingUser != nil {
		return errHTTPConflictUserExists
	}
	if err := s.userManager.AddUser(newAccount.Username, newAccount.Password, user.RoleUser); err != nil { // TODO this should return a User
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountGet(w http.ResponseWriter, _ *http.Request, v *visitor) error {
	info, err := v.Info()
	if err != nil {
		return err
	}
	limits, stats := info.Limits, info.Stats
	response := &apiAccountResponse{
		Limits: &apiAccountLimits{
			Basis:                    string(limits.Basis),
			Messages:                 limits.MessageLimit,
			MessagesExpiryDuration:   int64(limits.MessageExpiryDuration.Seconds()),
			Emails:                   limits.EmailLimit,
			Reservations:             limits.ReservationsLimit,
			AttachmentTotalSize:      limits.AttachmentTotalSizeLimit,
			AttachmentFileSize:       limits.AttachmentFileSizeLimit,
			AttachmentExpiryDuration: int64(limits.AttachmentExpiryDuration.Seconds()),
			AttachmentBandwidth:      limits.AttachmentBandwidthLimit,
		},
		Stats: &apiAccountStats{
			Messages:                     stats.Messages,
			MessagesRemaining:            stats.MessagesRemaining,
			Emails:                       stats.Emails,
			EmailsRemaining:              stats.EmailsRemaining,
			Reservations:                 stats.Reservations,
			ReservationsRemaining:        stats.ReservationsRemaining,
			AttachmentTotalSize:          stats.AttachmentTotalSize,
			AttachmentTotalSizeRemaining: stats.AttachmentTotalSizeRemaining,
		},
	}
	if v.user != nil {
		response.Username = v.user.Name
		response.Role = string(v.user.Role)
		response.SyncTopic = v.user.SyncTopic
		if v.user.Prefs != nil {
			if v.user.Prefs.Language != nil {
				response.Language = *v.user.Prefs.Language
			}
			if v.user.Prefs.Notification != nil {
				response.Notification = v.user.Prefs.Notification
			}
			if v.user.Prefs.Subscriptions != nil {
				response.Subscriptions = v.user.Prefs.Subscriptions
			}
		}
		if v.user.Tier != nil {
			response.Tier = &apiAccountTier{
				Code: v.user.Tier.Code,
				Name: v.user.Tier.Name,
			}
		}
		if v.user.Billing.StripeCustomerID != "" {
			response.Billing = &apiAccountBilling{
				Customer:     true,
				Subscription: v.user.Billing.StripeSubscriptionID != "",
				Status:       string(v.user.Billing.StripeSubscriptionStatus),
				PaidUntil:    v.user.Billing.StripeSubscriptionPaidUntil.Unix(),
				CancelAt:     v.user.Billing.StripeSubscriptionCancelAt.Unix(),
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
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleAccountDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountDeleteRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	} else if req.Password == "" {
		return errHTTPBadRequest
	}
	if _, err := s.userManager.Authenticate(v.user.Name, req.Password); err != nil {
		return errHTTPBadRequestIncorrectPasswordConfirmation
	}
	if v.user.Billing.StripeSubscriptionID != "" {
		log.Info("%s Canceling billing subscription %s", logHTTPPrefix(v, r), v.user.Billing.StripeSubscriptionID)
		if _, err := s.stripe.CancelSubscription(v.user.Billing.StripeSubscriptionID); err != nil {
			return err
		}
	}
	if err := s.maybeRemoveMessagesAndExcessReservations(logHTTPPrefix(v, r), v.user, 0); err != nil {
		return err
	}
	log.Info("%s Marking user %s as deleted", logHTTPPrefix(v, r), v.user.Name)
	if err := s.userManager.MarkUserRemoved(v.user); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountPasswordChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountPasswordChangeRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	} else if req.Password == "" || req.NewPassword == "" {
		return errHTTPBadRequest
	}
	if _, err := s.userManager.Authenticate(v.user.Name, req.Password); err != nil {
		return errHTTPBadRequestIncorrectPasswordConfirmation
	}
	if err := s.userManager.ChangePassword(v.user.Name, req.NewPassword); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountTokenIssue(w http.ResponseWriter, _ *http.Request, v *visitor) error {
	// TODO rate limit
	token, err := s.userManager.CreateToken(v.user)
	if err != nil {
		return err
	}
	response := &apiAccountTokenResponse{
		Token:   token.Value,
		Expires: token.Expires.Unix(),
	}
	return s.writeJSON(w, response)
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
	response := &apiAccountTokenResponse{
		Token:   token.Value,
		Expires: token.Expires.Unix(),
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleAccountTokenDelete(w http.ResponseWriter, _ *http.Request, v *visitor) error {
	// TODO rate limit
	if v.user.Token == "" {
		return errHTTPBadRequestNoTokenProvided
	}
	if err := s.userManager.RemoveToken(v.user); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
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
	if newPrefs.Language != nil {
		prefs.Language = newPrefs.Language
	}
	if newPrefs.Notification != nil {
		if prefs.Notification == nil {
			prefs.Notification = &user.NotificationPrefs{}
		}
		if newPrefs.Notification.DeleteAfter != nil {
			prefs.Notification.DeleteAfter = newPrefs.Notification.DeleteAfter
		}
		if newPrefs.Notification.Sound != nil {
			prefs.Notification.Sound = newPrefs.Notification.Sound
		}
		if newPrefs.Notification.MinPriority != nil {
			prefs.Notification.MinPriority = newPrefs.Notification.MinPriority
		}
	}
	if err := s.userManager.ChangeSettings(v.user); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
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
		newSubscription.ID = util.RandomStringPrefix(subscriptionIDPrefix, subscriptionIDLength)
		v.user.Prefs.Subscriptions = append(v.user.Prefs.Subscriptions, newSubscription)
		if err := s.userManager.ChangeSettings(v.user); err != nil {
			return err
		}
	}
	return s.writeJSON(w, newSubscription)
}

func (s *Server) handleAccountSubscriptionChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := apiAccountSubscriptionSingleRegex.FindStringSubmatch(r.URL.Path)
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
	return s.writeJSON(w, subscription)
}

func (s *Server) handleAccountSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := apiAccountSubscriptionSingleRegex.FindStringSubmatch(r.URL.Path)
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
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountReservationAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user != nil && v.user.Role == user.RoleAdmin {
		return errHTTPBadRequestMakesNoSenseForAdmin
	}
	req, err := readJSONWithLimit[apiAccountReservationRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	if !topicRegex.MatchString(req.Topic) {
		return errHTTPBadRequestTopicInvalid
	}
	everyone, err := user.ParsePermission(req.Everyone)
	if err != nil {
		return errHTTPBadRequestPermissionInvalid
	}
	if v.user.Tier == nil {
		return errHTTPUnauthorized
	}
	// CHeck if we are allowed to reserve this topic
	if err := s.userManager.CheckAllowAccess(v.user.Name, req.Topic); err != nil {
		return errHTTPConflictTopicReserved
	}
	hasReservation, err := s.userManager.HasReservation(v.user.Name, req.Topic)
	if err != nil {
		return err
	}
	if !hasReservation {
		reservations, err := s.userManager.ReservationsCount(v.user.Name)
		if err != nil {
			return err
		} else if reservations >= v.user.Tier.ReservationLimit {
			return errHTTPTooManyRequestsLimitReservations
		}
	}
	// Actually add the reservation
	if err := s.userManager.AddReservation(v.user.Name, req.Topic, everyone); err != nil {
		return err
	}
	// Kill existing subscribers
	t, err := s.topicFromID(req.Topic)
	if err != nil {
		return err
	}
	t.CancelSubscribers(v.user.ID)
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountReservationDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := apiAccountReservationSingleRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	topic := matches[1]
	if !topicRegex.MatchString(topic) {
		return errHTTPBadRequestTopicInvalid
	}
	authorized, err := s.userManager.HasReservation(v.user.Name, topic)
	if err != nil {
		return err
	} else if !authorized {
		return errHTTPUnauthorized
	}
	if err := s.userManager.RemoveReservations(v.user.Name, topic); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// maybeRemoveMessagesAndExcessReservations deletes topic reservations for the given user (if too many for tier),
// and marks associated messages for the topics as deleted. This also eventually deletes attachments.
// The process relies on the manager to perform the actual deletions (see runManager).
func (s *Server) maybeRemoveMessagesAndExcessReservations(logPrefix string, u *user.User, reservationsLimit int64) error {
	reservations, err := s.userManager.Reservations(u.Name)
	if err != nil {
		return err
	} else if int64(len(reservations)) <= reservationsLimit {
		return nil
	}
	topics := make([]string, 0)
	for i := int64(len(reservations)) - 1; i >= reservationsLimit; i-- {
		topics = append(topics, reservations[i].Topic)
	}
	log.Info("%s Removing excess reservations for topics %s", logPrefix, strings.Join(topics, ", "))
	if err := s.userManager.RemoveReservations(u.Name, topics...); err != nil {
		return err
	}
	if err := s.messageCache.ExpireMessages(topics...); err != nil {
		return err
	}
	return nil
}

func (s *Server) publishSyncEvent(v *visitor) error {
	if v.user == nil || v.user.SyncTopic == "" {
		return nil
	}
	log.Trace("Publishing sync event to user %s's sync topic %s", v.user.Name, v.user.SyncTopic)
	syncTopic, err := s.topicFromID(v.user.SyncTopic)
	if err != nil {
		return err
	}
	messageBytes, err := json.Marshal(&apiAccountSyncTopicResponse{Event: syncTopicAccountSyncEvent})
	if err != nil {
		return err
	}
	m := newDefaultMessage(syncTopic.ID, string(messageBytes))
	if err := syncTopic.Publish(v, m); err != nil {
		return err
	}
	return nil
}

func (s *Server) publishSyncEventAsync(v *visitor) {
	go func() {
		u := v.User()
		if u == nil || u.SyncTopic == "" {
			return
		}
		if err := s.publishSyncEvent(v); err != nil {
			log.Trace("Error publishing to user %s's sync topic %s: %s", u.Name, u.SyncTopic, err.Error())
		}
	}()
}
