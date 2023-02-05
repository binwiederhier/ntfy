package server

import (
	"encoding/json"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const (
	subscriptionIDLength      = 16
	subscriptionIDPrefix      = "su_"
	syncTopicAccountSyncEvent = "sync"
	tokenExpiryDuration       = 72 * time.Hour // Extend tokens by this much
)

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	if !u.Admin() { // u may be nil, but that's fine
		if !s.config.EnableSignup {
			return errHTTPBadRequestSignupNotEnabled
		} else if u != nil {
			return errHTTPUnauthorized // Cannot create account from user context
		}
		if !v.AccountCreationAllowed() {
			return errHTTPTooManyRequestsLimitAccountCreation
		}
	}
	newAccount, err := readJSONWithLimit[apiAccountCreateRequest](r.Body, jsonBodyBytesLimit, false)
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
	u := v.User()
	if u != nil {
		response.Username = u.Name
		response.Role = string(u.Role)
		response.SyncTopic = u.SyncTopic
		if u.Prefs != nil {
			if u.Prefs.Language != nil {
				response.Language = *u.Prefs.Language
			}
			if u.Prefs.Notification != nil {
				response.Notification = u.Prefs.Notification
			}
			if u.Prefs.Subscriptions != nil {
				response.Subscriptions = u.Prefs.Subscriptions
			}
		}
		if u.Tier != nil {
			response.Tier = &apiAccountTier{
				Code: u.Tier.Code,
				Name: u.Tier.Name,
			}
		}
		if u.Billing.StripeCustomerID != "" {
			response.Billing = &apiAccountBilling{
				Customer:     true,
				Subscription: u.Billing.StripeSubscriptionID != "",
				Status:       string(u.Billing.StripeSubscriptionStatus),
				PaidUntil:    u.Billing.StripeSubscriptionPaidUntil.Unix(),
				CancelAt:     u.Billing.StripeSubscriptionCancelAt.Unix(),
			}
		}
		reservations, err := s.userManager.Reservations(u.Name)
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
		tokens, err := s.userManager.Tokens(u.ID)
		if err != nil {
			return err
		}
		if len(tokens) > 0 {
			response.Tokens = make([]*apiAccountTokenResponse, 0)
			for _, t := range tokens {
				var lastOrigin string
				if t.LastOrigin != netip.IPv4Unspecified() {
					lastOrigin = t.LastOrigin.String()
				}
				response.Tokens = append(response.Tokens, &apiAccountTokenResponse{
					Token:      t.Value,
					Label:      t.Label,
					LastAccess: t.LastAccess.Unix(),
					LastOrigin: lastOrigin,
					Expires:    t.Expires.Unix(),
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
	req, err := readJSONWithLimit[apiAccountDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if req.Password == "" {
		return errHTTPBadRequest
	}
	u := v.User()
	if _, err := s.userManager.Authenticate(u.Name, req.Password); err != nil {
		return errHTTPBadRequestIncorrectPasswordConfirmation
	}
	if u.Billing.StripeSubscriptionID != "" {
		logvr(v, r).Tag(tagPay).Info("Canceling billing subscription for user %s", u.Name)
		if _, err := s.stripe.CancelSubscription(u.Billing.StripeSubscriptionID); err != nil {
			return err
		}
	}
	if err := s.maybeRemoveMessagesAndExcessReservations(r, v, u, 0); err != nil {
		return err
	}
	logvr(v, r).Tag(tagAccount).Info("Marking user %s as deleted", u.Name)
	if err := s.userManager.MarkUserRemoved(u); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountPasswordChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountPasswordChangeRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if req.Password == "" || req.NewPassword == "" {
		return errHTTPBadRequest
	}
	u := v.User()
	if _, err := s.userManager.Authenticate(u.Name, req.Password); err != nil {
		return errHTTPBadRequestIncorrectPasswordConfirmation
	}
	if err := s.userManager.ChangePassword(u.Name, req.NewPassword); err != nil {
		return err
	}
	logvr(v, r).Tag(tagAccount).Debug("Changed password for user %s", u.Name)
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountTokenCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// TODO rate limit
	req, err := readJSONWithLimit[apiAccountTokenIssueRequest](r.Body, jsonBodyBytesLimit, true) // Allow empty body!
	if err != nil {
		return err
	}
	var label string
	if req.Label != nil {
		label = *req.Label
	}
	expires := time.Now().Add(tokenExpiryDuration)
	if req.Expires != nil {
		expires = time.Unix(*req.Expires, 0)
	}
	u := v.User()
	token, err := s.userManager.CreateToken(u.ID, label, expires, v.IP())
	if err != nil {
		return err
	}
	logvr(v, r).Tag(tagAccount).Debug("Created token for user %s", u.Name)
	response := &apiAccountTokenResponse{
		Token:      token.Value,
		Label:      token.Label,
		LastAccess: token.LastAccess.Unix(),
		LastOrigin: token.LastOrigin.String(),
		Expires:    token.Expires.Unix(),
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleAccountTokenUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// TODO rate limit
	u := v.User()
	req, err := readJSONWithLimit[apiAccountTokenUpdateRequest](r.Body, jsonBodyBytesLimit, true) // Allow empty body!
	if err != nil {
		return err
	} else if req.Token == "" {
		req.Token = u.Token
		if req.Token == "" {
			return errHTTPBadRequestNoTokenProvided
		}
	}
	var expires *time.Time
	if req.Expires != nil {
		expires = util.Time(time.Unix(*req.Expires, 0))
	} else if req.Label == nil {
		// If label and expires are not set, simply extend the token by 72 hours
		expires = util.Time(time.Now().Add(tokenExpiryDuration))
	}
	token, err := s.userManager.ChangeToken(u.ID, req.Token, req.Label, expires)
	if err != nil {
		return err
	}
	response := &apiAccountTokenResponse{
		Token:      token.Value,
		Label:      token.Label,
		LastAccess: token.LastAccess.Unix(),
		LastOrigin: token.LastOrigin.String(),
		Expires:    token.Expires.Unix(),
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleAccountTokenDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// TODO rate limit
	u := v.User()
	token := readParam(r, "X-Token", "Token") // DELETEs cannot have a body, and we don't want it in the path
	if token == "" {
		token = u.Token
		if token == "" {
			return errHTTPBadRequestNoTokenProvided
		}
	}
	if err := s.userManager.RemoveToken(u.ID, token); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountSettingsChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	newPrefs, err := readJSONWithLimit[user.Prefs](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u := v.User()
	if u.Prefs == nil {
		u.Prefs = &user.Prefs{}
	}
	prefs := u.Prefs
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
	if err := s.userManager.ChangeSettings(u); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountSubscriptionAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	newSubscription, err := readJSONWithLimit[user.Subscription](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u := v.User()
	if u.Prefs == nil {
		u.Prefs = &user.Prefs{}
	}
	newSubscription.ID = "" // Client cannot set ID
	for _, subscription := range u.Prefs.Subscriptions {
		if newSubscription.BaseURL == subscription.BaseURL && newSubscription.Topic == subscription.Topic {
			newSubscription = subscription
			break
		}
	}
	if newSubscription.ID == "" {
		newSubscription.ID = util.RandomStringPrefix(subscriptionIDPrefix, subscriptionIDLength)
		u.Prefs.Subscriptions = append(u.Prefs.Subscriptions, newSubscription)
		if err := s.userManager.ChangeSettings(u); err != nil {
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
	updatedSubscription, err := readJSONWithLimit[user.Subscription](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u := v.User()
	if u.Prefs == nil || u.Prefs.Subscriptions == nil {
		return errHTTPNotFound
	}
	var subscription *user.Subscription
	for _, sub := range u.Prefs.Subscriptions {
		if sub.ID == subscriptionID {
			sub.DisplayName = updatedSubscription.DisplayName
			subscription = sub
			break
		}
	}
	if subscription == nil {
		return errHTTPNotFound
	}
	if err := s.userManager.ChangeSettings(u); err != nil {
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
	u := v.User()
	if u.Prefs == nil || u.Prefs.Subscriptions == nil {
		return nil
	}
	newSubscriptions := make([]*user.Subscription, 0)
	for _, subscription := range u.Prefs.Subscriptions {
		if subscription.ID != subscriptionID {
			newSubscriptions = append(newSubscriptions, subscription)
		}
	}
	if len(newSubscriptions) < len(u.Prefs.Subscriptions) {
		u.Prefs.Subscriptions = newSubscriptions
		if err := s.userManager.ChangeSettings(u); err != nil {
			return err
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountReservationAdd adds a topic reservation for the logged-in user, but only if the user has a tier
// with enough remaining reservations left, or if the user is an admin. Admins can always reserve a topic, unless
// it is already reserved by someone else.
func (s *Server) handleAccountReservationAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[apiAccountReservationRequest](r.Body, jsonBodyBytesLimit, false)
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
	// Check if we are allowed to reserve this topic
	if u.User() && u.Tier == nil {
		return errHTTPUnauthorized
	} else if err := s.userManager.CheckAllowAccess(u.Name, req.Topic); err != nil {
		return errHTTPConflictTopicReserved
	} else if u.User() {
		hasReservation, err := s.userManager.HasReservation(u.Name, req.Topic)
		if err != nil {
			return err
		}
		if !hasReservation {
			reservations, err := s.userManager.ReservationsCount(u.Name)
			if err != nil {
				return err
			} else if reservations >= u.Tier.ReservationLimit {
				return errHTTPTooManyRequestsLimitReservations
			}
		}
	}
	// Actually add the reservation
	if err := s.userManager.AddReservation(u.Name, req.Topic, everyone); err != nil {
		return err
	}
	// Kill existing subscribers
	t, err := s.topicFromID(req.Topic)
	if err != nil {
		return err
	}
	t.CancelSubscribers(u.ID)
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountReservationDelete deletes a topic reservation if it is owned by the current user
func (s *Server) handleAccountReservationDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := apiAccountReservationSingleRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	topic := matches[1]
	if !topicRegex.MatchString(topic) {
		return errHTTPBadRequestTopicInvalid
	}
	u := v.User()
	authorized, err := s.userManager.HasReservation(u.Name, topic)
	if err != nil {
		return err
	} else if !authorized {
		return errHTTPUnauthorized
	}
	if err := s.userManager.RemoveReservations(u.Name, topic); err != nil {
		return err
	}
	deleteMessages := readBoolParam(r, false, "X-Delete-Messages", "Delete-Messages")
	if deleteMessages {
		if err := s.messageCache.ExpireMessages(topic); err != nil {
			return err
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

// maybeRemoveMessagesAndExcessReservations deletes topic reservations for the given user (if too many for tier),
// and marks associated messages for the topics as deleted. This also eventually deletes attachments.
// The process relies on the manager to perform the actual deletions (see runManager).
func (s *Server) maybeRemoveMessagesAndExcessReservations(r *http.Request, v *visitor, u *user.User, reservationsLimit int64) error {
	reservations, err := s.userManager.Reservations(u.Name)
	if err != nil {
		return err
	} else if int64(len(reservations)) <= reservationsLimit {
		logvr(v, r).Tag(tagAccount).Debug("No excess reservations to remove")
		return nil
	}
	topics := make([]string, 0)
	for i := int64(len(reservations)) - 1; i >= reservationsLimit; i-- {
		topics = append(topics, reservations[i].Topic)
	}
	logvr(v, r).Tag(tagAccount).Info("Removing excess reservations for topics %s", strings.Join(topics, ", "))
	if err := s.userManager.RemoveReservations(u.Name, topics...); err != nil {
		return err
	}
	if err := s.messageCache.ExpireMessages(topics...); err != nil {
		return err
	}
	return nil
}

// publishSyncEventAsync kicks of a Go routine to publish a sync message to the user's sync topic
func (s *Server) publishSyncEventAsync(v *visitor) {
	go func() {
		if err := s.publishSyncEvent(v); err != nil {
			log.Trace("%s Error publishing to user's sync topic: %s", v.String(), err.Error())
		}
	}()
}

// publishSyncEvent publishes a sync message to the user's sync topic
func (s *Server) publishSyncEvent(v *visitor) error {
	u := v.User()
	if u == nil || u.SyncTopic == "" {
		return nil
	}
	log.Trace("Publishing sync event to user %s's sync topic %s", u.Name, u.SyncTopic)
	syncTopic, err := s.topicFromID(u.SyncTopic)
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
