package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/twilio"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

const (
	syncTopicAccountSyncEvent    = "sync"
	tokenExpiryDuration          = 72 * time.Hour // Extend tokens by this much
	emailVerificationTokenExpiry = 24 * time.Hour // Magic-link lifetime for email verification
	passwordResetTokenExpiry     = time.Hour      // Magic-link lifetime for password reset (higher-privilege -> shorter)
)

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	if !u.IsAdmin() { // u may be nil, but that's fine
		if !s.config.EnableSignup {
			return errHTTPBadRequestSignupNotEnabled
		} else if u != nil {
			return errHTTPUnauthorized // Cannot create account from user context
		}
		if !v.AccountActionAllowed() {
			return errHTTPTooManyRequestsLimitAccountActions
		}
	}
	newAccount, err := readJSONWithLimit[apiAccountCreateRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	if newAccount.Email != "" && !emailAddressRegex.MatchString(newAccount.Email) {
		return errHTTPBadRequestEmailAddressInvalid
	}
	if existingUser, _ := s.userManager.User(newAccount.Username); existingUser != nil {
		return errHTTPConflictUserExists
	}
	logvr(v, r).Tag(tagAccount).Field("user_name", newAccount.Username).Info("Creating user %s", newAccount.Username)
	if err := s.userManager.AddUser(newAccount.Username, newAccount.Password, user.RoleUser, false); err != nil {
		if errors.Is(err, user.ErrInvalidArgument) {
			return errHTTPBadRequestInvalidUsername
		}
		return err
	}
	v.AccountActionPerformed()
	// If an email was provided and email sending is configured, start verification (best-effort).
	// The address becomes the primary email on verify (the new account has no primary yet); a
	// failure to send must not fail signup, so we only log it.
	if newAccount.Email != "" && s.mailer != nil {
		if u, err := s.userManager.User(newAccount.Username); err != nil {
			logvr(v, r).Tag(tagAccount).Err(err).Warn("Failed to load new user for email verification")
		} else if err := s.enqueueEmailVerification(u.ID, newAccount.Email); err != nil {
			logvr(v, r).Tag(tagAccount).Err(err).Warn("Failed to send signup email verification")
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	info, err := v.Info()
	if err != nil {
		return err
	}
	logvr(v, r).Tag(tagAccount).Fields(visitorExtendedInfoContext(info)).Debug("Retrieving account stats")
	limits, stats := info.Limits, info.Stats
	response := &apiAccountResponse{
		Limits: &apiAccountLimits{
			Basis:                    string(limits.Basis),
			Messages:                 limits.MessageLimit,
			MessagesExpiryDuration:   int64(limits.MessageExpiryDuration.Seconds()),
			Emails:                   limits.EmailLimit,
			Calls:                    limits.CallLimit,
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
			Calls:                        stats.Calls,
			CallsRemaining:               stats.CallsRemaining,
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
		response.Provisioned = u.Provisioned
		if u.Prefs != nil {
			if u.Prefs.Language != nil {
				response.Language = *u.Prefs.Language
			}
			if u.Prefs.DateFormat != nil {
				response.DateFormat = *u.Prefs.DateFormat
			}
			if u.Prefs.TimeFormat != nil {
				response.TimeFormat = *u.Prefs.TimeFormat
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
				Interval:     string(u.Billing.StripeSubscriptionInterval),
				PaidUntil:    u.Billing.StripeSubscriptionPaidUntil.Unix(),
				CancelAt:     u.Billing.StripeSubscriptionCancelAt.Unix(),
			}
		}
		if s.config.EnableReservations {
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
					Token:       t.Value,
					Label:       t.Label,
					LastAccess:  t.LastAccess.Unix(),
					LastOrigin:  lastOrigin,
					Expires:     t.Expires.Unix(),
					Provisioned: t.Provisioned,
				})
			}
		}
		if s.config.TwilioAccount != "" {
			phoneNumbers, err := s.userManager.PhoneNumbers(u.ID)
			if err != nil {
				return err
			}
			if len(phoneNumbers) > 0 {
				response.PhoneNumbers = phoneNumbers
			}
		}
		if s.mailer != nil {
			emails, err := s.userManager.Emails(u.ID)
			if err != nil {
				return err
			}
			pendingEmails, err := s.userManager.PendingEmails(u.ID)
			if err != nil {
				return err
			}
			// Combine verified (with primary flag) and pending (unverified) into one list
			emailInfos := make([]*apiAccountEmailInfo, 0, len(emails)+len(pendingEmails))
			for _, email := range emails {
				emailInfos = append(emailInfos, &apiAccountEmailInfo{Address: email.Address, Primary: email.Primary})
			}
			for _, email := range pendingEmails {
				emailInfos = append(emailInfos, &apiAccountEmailInfo{Address: email, Pending: true})
			}
			if len(emailInfos) > 0 {
				response.Emails = emailInfos
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
	if err := s.userManager.CanChangeUser(u.Name); err != nil {
		if errors.Is(err, user.ErrProvisionedUserChange) {
			return errHTTPConflictProvisionedUserChange
		}
		return err
	}
	if s.webPush != nil && u.ID != "" {
		if err := s.webPush.RemoveSubscriptionsByUserID(u.ID); err != nil {
			logvr(v, r).Err(err).Warn("Error removing web push subscriptions for %s", u.Name)
		}
	}
	if u.Billing.StripeSubscriptionID != "" {
		logvr(v, r).Tag(tagStripe).Info("Canceling billing subscription for user %s", u.Name)
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
	logvr(v, r).Tag(tagAccount).Debug("Changing password for user %s", u.Name)
	if err := s.userManager.ChangePassword(u.Name, req.NewPassword, false); err != nil {
		if errors.Is(err, user.ErrProvisionedUserChange) {
			return errHTTPConflictProvisionedUserChange
		}
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountLogin authenticates a username-or-email + password (via the ensureUser wrapper's
// Basic Auth), mints a session token, and returns it together with the canonical username. Unlike
// the token endpoint (which exists to mint arbitrary API tokens), this endpoint's job is to log a
// user in, so it also reports who they are (the identifier they typed may be a primary email).
func (s *Server) handleAccountLogin(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	logvr(v, r).Tag(tagAccount).Info("Logging in user %s", u.Name)
	token, err := s.userManager.CreateToken(u.ID, "", time.Now().Add(tokenExpiryDuration), v.IP(), false)
	if err != nil {
		return err
	}
	response := &apiAccountLoginResponse{
		Token:    token.Value,
		Username: u.Name,
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleAccountTokenCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
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
	logvr(v, r).
		Tag(tagAccount).
		Fields(log.Context{
			"token_label":   label,
			"token_expires": expires,
		}).
		Debug("Creating token for user %s", u.Name)
	token, err := s.userManager.CreateToken(u.ID, label, expires, v.IP(), false)
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

func (s *Server) handleAccountTokenUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
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
		expires = util.Time(time.Now().Add(tokenExpiryDuration)) // If label/expires not set, extend token by 72 hours
	}
	logvr(v, r).
		Tag(tagAccount).
		Fields(log.Context{
			"token_label":   req.Label,
			"token_expires": expires,
		}).
		Debug("Updating token for user %s as deleted", u.Name)
	token, err := s.userManager.ChangeToken(u.ID, req.Token, req.Label, expires)
	if err != nil {
		if errors.Is(err, user.ErrProvisionedTokenChange) {
			return errHTTPConflictProvisionedTokenChange
		}
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
	u := v.User()
	token := readParam(r, "X-Token", "Token") // DELETEs cannot have a body, and we don't want it in the path
	if token == "" {
		token = u.Token
		if token == "" {
			return errHTTPBadRequestNoTokenProvided
		}
	}
	if err := s.userManager.RemoveToken(u.ID, token); err != nil {
		if errors.Is(err, user.ErrProvisionedTokenChange) {
			return errHTTPConflictProvisionedTokenChange
		}
		return err
	}
	logvr(v, r).
		Tag(tagAccount).
		Field("token", token).
		Debug("Deleted token for user %s", u.Name)
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
	if newPrefs.DateFormat != nil {
		prefs.DateFormat = newPrefs.DateFormat
	}
	if newPrefs.TimeFormat != nil {
		prefs.TimeFormat = newPrefs.TimeFormat
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
	logvr(v, r).Tag(tagAccount).Debug("Changing account settings for user %s", u.Name)
	if err := s.userManager.ChangeSettings(u.ID, prefs); err != nil {
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
	prefs := u.Prefs
	if prefs == nil {
		prefs = &user.Prefs{}
	}
	for _, subscription := range prefs.Subscriptions {
		if newSubscription.BaseURL == subscription.BaseURL && newSubscription.Topic == subscription.Topic {
			return errHTTPConflictSubscriptionExists
		}
	}
	prefs.Subscriptions = append(prefs.Subscriptions, newSubscription)
	logvr(v, r).Tag(tagAccount).With(newSubscription).Debug("Adding subscription for user %s", u.Name)
	if err := s.userManager.ChangeSettings(u.ID, prefs); err != nil {
		return err
	}
	return s.writeJSON(w, newSubscription)
}

func (s *Server) handleAccountSubscriptionChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	updatedSubscription, err := readJSONWithLimit[user.Subscription](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u := v.User()
	prefs := u.Prefs
	if prefs == nil || prefs.Subscriptions == nil {
		return errHTTPNotFound
	}
	var subscription *user.Subscription
	for _, sub := range prefs.Subscriptions {
		if sub.BaseURL == updatedSubscription.BaseURL && sub.Topic == updatedSubscription.Topic {
			sub.DisplayName = updatedSubscription.DisplayName
			subscription = sub
			break
		}
	}
	if subscription == nil {
		return errHTTPNotFound
	}
	logvr(v, r).Tag(tagAccount).With(subscription).Debug("Changing subscription for user %s", u.Name)
	if err := s.userManager.ChangeSettings(u.ID, prefs); err != nil {
		return err
	}
	return s.writeJSON(w, subscription)
}

func (s *Server) handleAccountSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// DELETEs cannot have a body, and we don't want it in the path
	deleteBaseURL := readParam(r, "X-BaseURL", "BaseURL")
	deleteTopic := readParam(r, "X-Topic", "Topic")
	u := v.User()
	prefs := u.Prefs
	if prefs == nil || prefs.Subscriptions == nil {
		return nil
	}
	newSubscriptions := make([]*user.Subscription, 0)
	for _, sub := range u.Prefs.Subscriptions {
		if sub.BaseURL == deleteBaseURL && sub.Topic == deleteTopic {
			logvr(v, r).Tag(tagAccount).With(sub).Debug("Removing subscription for user %s", u.Name)
		} else {
			newSubscriptions = append(newSubscriptions, sub)
		}
	}
	if len(newSubscriptions) < len(prefs.Subscriptions) {
		prefs.Subscriptions = newSubscriptions
		if err := s.userManager.ChangeSettings(u.ID, prefs); err != nil {
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
	if u.IsUser() && u.Tier == nil {
		return errHTTPUnauthorized
	} else if err := s.userManager.AllowReservation(u.Name, req.Topic); err != nil {
		return errHTTPConflictTopicReserved
	}
	// Actually add the reservation (with limit check inside the transaction to avoid races)
	logvr(v, r).
		Tag(tagAccount).
		Fields(log.Context{
			"topic":    req.Topic,
			"everyone": everyone.String(),
		}).
		Debug("Adding topic reservation")
	var limit int64
	if u.IsUser() && u.Tier != nil {
		limit = u.Tier.ReservationLimit
	}
	if err := s.userManager.AddReservation(u.Name, req.Topic, everyone, limit); err != nil {
		if errors.Is(err, user.ErrTooManyReservations) {
			return errHTTPTooManyRequestsLimitReservations
		}
		return err
	}
	// Kill existing subscribers
	t, err := s.topicFromID(v, req.Topic)
	if err != nil {
		return err
	}
	t.CancelSubscribersExceptUser(u.ID)
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
	deleteMessages := readBoolParam(r, false, "X-Delete-Messages", "Delete-Messages")
	logvr(v, r).
		Tag(tagAccount).
		Fields(log.Context{
			"topic":           topic,
			"delete_messages": deleteMessages,
		}).
		Debug("Removing topic reservation")
	if err := s.userManager.RemoveReservations(u.Name, topic); err != nil {
		return err
	}
	if deleteMessages {
		if err := s.messageCache.ExpireMessages(topic); err != nil {
			return err
		}
		s.pruneMessages()
	}
	return s.writeJSON(w, newSuccessResponse())
}

// maybeRemoveMessagesAndExcessReservations deletes topic reservations for the given user (if too many for tier),
// and marks associated messages for the topics as deleted. This also eventually deletes attachments.
// The process relies on the manager to perform the actual deletions (see runManager).
func (s *Server) maybeRemoveMessagesAndExcessReservations(r *http.Request, v *visitor, u *user.User, reservationsLimit int64) error {
	removedTopics, err := s.userManager.RemoveExcessReservations(u.Name, reservationsLimit)
	if err != nil {
		return err
	} else if len(removedTopics) == 0 {
		logvr(v, r).Tag(tagAccount).Debug("No excess reservations to remove")
		return nil
	}
	logvr(v, r).Tag(tagAccount).Info("Removed excess topic reservations, now removing messages for topics %s", strings.Join(removedTopics, ", "))
	if err := s.messageCache.ExpireMessages(removedTopics...); err != nil {
		return err
	}
	go s.pruneMessages()
	return nil
}

func (s *Server) handleAccountPhoneNumberVerify(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[apiAccountPhoneNumberVerifyRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !phoneNumberRegex.MatchString(req.Number) {
		return errHTTPBadRequestPhoneNumberInvalid
	} else if req.Channel != "sms" && req.Channel != "call" {
		return errHTTPBadRequestPhoneNumberVerifyChannelInvalid
	}
	// Check user is allowed to add phone numbers
	if u == nil || (u.IsUser() && u.Tier == nil) {
		return errHTTPUnauthorized
	} else if u.IsUser() && u.Tier.CallLimit == 0 {
		return errHTTPUnauthorized
	}
	// Check if phone number exists
	phoneNumbers, err := s.userManager.PhoneNumbers(u.ID)
	if err != nil {
		return err
	} else if util.Contains(phoneNumbers, req.Number) {
		return errHTTPConflictPhoneNumberExists
	}
	// Actually add the unverified number, and send verification
	logvr(v, r).Tag(tagAccount).Field("phone_number", req.Number).Debug("Sending phone number verification")
	if err := s.twilio.Verify(req.Number, req.Channel); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountPhoneNumberAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[apiAccountPhoneNumberAddRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	if !phoneNumberRegex.MatchString(req.Number) {
		return errHTTPBadRequestPhoneNumberInvalid
	}
	if err := s.twilio.CheckVerify(req.Number, req.Code); err != nil {
		if errors.Is(err, twilio.ErrVerificationExpired) {
			return errHTTPGonePhoneVerificationExpired
		}
		return err
	}
	logvr(v, r).Tag(tagAccount).Field("phone_number", req.Number).Debug("Adding phone number as verified")
	if err := s.userManager.AddPhoneNumber(u.ID, req.Number); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccountPhoneNumberDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[apiAccountPhoneNumberAddRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	if !phoneNumberRegex.MatchString(req.Number) {
		return errHTTPBadRequestPhoneNumberInvalid
	}
	logvr(v, r).Tag(tagAccount).Field("phone_number", req.Number).Debug("Deleting phone number")
	if err := s.userManager.RemovePhoneNumber(u.ID, req.Number); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountEmailAdd starts email verification (PUT /v1/account/email): it generates a
// magic-link token, stores a pending verification, and emails the link. The address is NOT
// added to the verified list until the user clicks the link (handleAccountEmailVerify).
func (s *Server) handleAccountEmailAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[apiAccountEmailRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !emailAddressRegex.MatchString(req.Email) {
		return errHTTPBadRequestEmailAddressInvalid
	}
	// Check user is allowed to add emails (the tier email limit gates the feature)
	if u.IsUser() && u.Tier != nil && u.Tier.EmailLimit == 0 {
		return errHTTPUnauthorized
	} else if u.IsUser() && u.Tier == nil && s.config.VisitorEmailLimitBurst == 0 {
		return errHTTPUnauthorized
	}
	// Reject if already verified on this account (pending re-requests are fine -- they replace)
	emails, err := s.userManager.Emails(u.ID)
	if err != nil {
		return err
	} else if emails.Contains(req.Email) {
		return errHTTPConflictEmailExists
	}
	// Rate limit (counts against the user's email quota)
	if !v.EmailAllowed() {
		return errHTTPTooManyRequestsLimitEmails
	}
	logvr(v, r).Tag(tagAccount).Field("email", req.Email).Info("Starting email verification")
	if err := s.enqueueEmailVerification(u.ID, req.Email); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountEmailVerify performs verification from the (unauthenticated) landing page
// (POST /v1/account/email/verify): it validates the raw token, adds the address to the user's
// verified emails, and -- if the user has no primary yet -- promotes it. No auth is required;
// the token binds the action to a user, so the click works from a logged-out mail client.
func (s *Server) handleAccountEmailVerify(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountEmailVerifyRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if req.Token == "" {
		return errHTTPBadRequestEmailVerificationLinkInvalid
	}
	m, err := s.userManager.VerifyEmail(req.Token)
	if errors.Is(err, user.ErrMagicLinkNotFound) {
		return errHTTPBadRequestEmailVerificationLinkInvalid
	} else if err != nil {
		return err
	}
	logvr(v, r).Tag(tagAccount).Field("email", m.Email).Info("Email verified")
	// Refresh the verified user's other sessions. The request is unauthenticated (v.User() is
	// usually nil), so resolve the user from the token row and publish to their sync topic.
	s.publishSyncEventForUserIDAsync(v, m.UserID)
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountEmailDelete removes an email address, whether verified or still pending
// (DELETE /v1/account/email). Removing the primary leaves the account with no primary.
func (s *Server) handleAccountEmailDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[apiAccountEmailRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !emailAddressRegex.MatchString(req.Email) {
		return errHTTPBadRequestEmailAddressInvalid
	}
	logvr(v, r).Tag(tagAccount).Field("email", req.Email).Debug("Deleting email (verified or pending)")
	if err := s.userManager.RemoveEmail(u.ID, req.Email); err != nil {
		return err
	}
	// Also drop any pending verification for the address (no-op if there is none)
	if err := s.userManager.DeleteEmailVerification(u.ID, req.Email); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountEmailSetPrimary marks an already-verified email as the user's primary (recovery)
// email (POST /v1/account/email/primary).
func (s *Server) handleAccountEmailSetPrimary(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[apiAccountEmailRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !emailAddressRegex.MatchString(req.Email) {
		return errHTTPBadRequestEmailAddressInvalid
	}
	logvr(v, r).Tag(tagAccount).Field("email", req.Email).Info("Setting primary email")
	err = s.userManager.SetPrimaryEmail(u.ID, req.Email)
	if errors.Is(err, user.ErrEmailPrimaryElsewhere) {
		return errHTTPConflictEmailPrimaryElsewhere
	} else if errors.Is(err, user.ErrEmailNotFound) {
		return errHTTPBadRequestEmailAddressNotVerified
	} else if err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountEmailResend re-sends a pending email verification (POST /v1/account/email/resend).
func (s *Server) handleAccountEmailResend(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[apiAccountEmailRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !emailAddressRegex.MatchString(req.Email) {
		return errHTTPBadRequestEmailAddressInvalid
	}
	// Only resend for an address that is actually pending on this account
	pending, err := s.userManager.PendingEmails(u.ID)
	if err != nil {
		return err
	} else if !util.Contains(pending, req.Email) {
		return errHTTPBadRequestEmailAddressInvalid
	}
	if !v.EmailAllowed() {
		return errHTTPTooManyRequestsLimitEmails
	}
	logvr(v, r).Tag(tagAccount).Field("email", req.Email).Info("Resending email verification")
	if err := s.enqueueEmailVerification(u.ID, req.Email); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// enqueueEmailVerification generates a magic-link token for the given address, stores the
// pending verification (replacing any existing one), and emails the link. Shared by the add,
// resend, signup, and Stripe paths. Requires base-url to build an absolute link.
func (s *Server) enqueueEmailVerification(userID, email string) error {
	if s.config.BaseURL == "" {
		return errHTTPInternalErrorMissingBaseURL
	}
	token, err := s.userManager.AddMagicLink(user.MagicLinkKindEmailVerify, userID, email, emailVerificationTokenExpiry)
	if err != nil {
		return err
	}
	link := s.config.BaseURL + webAppEmailVerifyPathPrefix + token
	return s.mailer.SendEmailVerification(email, link)
}

// handleAccountPasswordResetRequest starts a password reset (POST /v1/account/password/reset/request,
// unauthenticated). It resolves the identifier (username or primary email) to at most one account
// and emails a reset link to that account's primary email. The response is always a uniform 200,
// regardless of whether anything matched, so it cannot be used to probe for accounts.
func (s *Server) handleAccountPasswordResetRequest(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountPasswordResetRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	// Rate limit via the shared per-visitor account-creation bucket (no new limiter/config)
	if !v.AccountActionAllowed() {
		return errHTTPTooManyRequestsLimitAccountActions
	}
	v.AccountActionPerformed() // Consume a token on every request (including no-match), to throttle probing
	identifier := strings.TrimSpace(req.Identifier)
	if identifier != "" && s.config.BaseURL != "" {
		if userID, email, ok := s.resolveResetPasswordTarget(identifier); ok {
			token, err := s.userManager.AddMagicLink(user.MagicLinkKindPasswordReset, userID, "", passwordResetTokenExpiry)
			if err != nil {
				logvr(v, r).Tag(tagAccount).Err(err).Warn("Failed to create password reset token")
			} else {
				link := s.config.BaseURL + webAppPasswordResetPathPrefix + token
				logvr(v, r).Tag(tagAccount).Field("user_id", userID).Info("Sending password reset link")
				if err := s.mailer.SendPasswordReset(email, link); err != nil {
					logvr(v, r).Tag(tagAccount).Err(err).Warn("Failed to send password reset email")
				}
			}
		} else {
			logvr(v, r).Tag(tagAccount).Debug("Password reset requested for unknown identifier (uniform response)")
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

// resolveResetPasswordTarget resolves a reset identifier (username or primary email) to a single account
// and its primary email. It applies the reset policy on top of the lookup: provisioned users are
// excluded, and ok=false is returned unless the account has a verified primary email (reset
// requires one, and that is where the link is sent).
func (s *Server) resolveResetPasswordTarget(identifier string) (userID string, email string, ok bool) {
	u, err := s.userManager.UserByEmailOrUsername(identifier)
	if err != nil || u == nil || u.Provisioned {
		return "", "", false
	}
	primary, err := s.userManager.PrimaryEmail(u.ID)
	if err != nil || primary == "" {
		return "", "", false
	}
	return u.ID, primary, true
}

// handleAccountPasswordReset performs the reset (POST /v1/account/password/reset, unauthenticated):
// it validates the token and sets the new password. Existing access tokens stay valid.
func (s *Server) handleAccountPasswordReset(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountPasswordResetConfirmRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	if req.Token == "" {
		return errHTTPBadRequestResetLinkInvalid
	} else if req.Password == "" {
		return errHTTPBadRequest
	}
	err = s.userManager.ResetPassword(req.Token, req.Password)
	if errors.Is(err, user.ErrMagicLinkNotFound) || errors.Is(err, user.ErrProvisionedUserChange) {
		return errHTTPBadRequestResetLinkInvalid // Generic 400 (provisioned users can't be reset; don't leak that)
	} else if err != nil {
		return err
	}
	logvr(v, r).Tag(tagAccount).Info("Password reset performed")
	return s.writeJSON(w, newSuccessResponse())
}

// convertEmailAddress resolves the X-Email value to the address ntfy should send to.
//
// "yes"/"true"/"1" resolves to the user's primary verified address -- or, if no primary is
// designated (e.g. a provisioned user), the first verified address (alphabetically). This is
// independent of smtp-sender-verify: it only requires an authenticated user with a verified
// address, since it means "send to my own email".
//
// A literal address is sent as-is when smtp-sender-verify is false (the default, backwards
// compatible); when true, the address must be one the user has verified.
func (s *Server) convertEmailAddress(u *user.User, email string) (string, *errHTTP) {
	if toBool(email) {
		if u == nil {
			return "", errHTTPBadRequestAnonymousEmailNotAllowed
		} else if s.userManager == nil {
			return "", errHTTPBadRequestEmailAddressNotVerified
		}
		primary, err := s.userManager.PrimaryEmail(u.ID)
		if err != nil {
			return "", errHTTPInternalError
		} else if primary != "" {
			return primary, nil
		}
		// No primary designated -> fall back to the first verified address, if any
		emails, err := s.userManager.Emails(u.ID)
		if err != nil {
			return "", errHTTPInternalError
		} else if len(emails) > 0 {
			return emails[0].Address, nil
		}
		return "", errHTTPBadRequestEmailAddressNotVerified
	}
	// A literal address
	if !s.config.SMTPSenderVerify {
		return email, nil
	} else if u == nil {
		return "", errHTTPBadRequestAnonymousEmailNotAllowed
	} else if s.userManager == nil {
		return email, nil
	}
	emails, err := s.userManager.Emails(u.ID)
	if err != nil {
		return "", errHTTPInternalError
	} else if emails.Contains(email) {
		return email, nil
	}
	return "", errHTTPBadRequestEmailAddressNotVerified
}

// publishSyncEventAsync kicks of a Go routine to publish a sync message to the user's sync topic
func (s *Server) publishSyncEventAsync(v *visitor) {
	go func() {
		if err := s.publishSyncEvent(v); err != nil {
			logv(v).Err(err).Trace("Error publishing to user's sync topic")
		}
	}()
}

// publishSyncEvent publishes a sync message to the authenticated user's sync topic
func (s *Server) publishSyncEvent(v *visitor) error {
	return s.publishSyncEventForUser(v, v.User())
}

// publishSyncEventForUserIDAsync publishes a sync event to the sync topic of the user with the
// given ID, resolving the user first. Used by the unauthenticated email-verify handler, where
// the request visitor has no associated user but the token identifies the account to refresh.
func (s *Server) publishSyncEventForUserIDAsync(v *visitor, userID string) {
	go func() {
		u, err := s.userManager.UserByID(userID)
		if err != nil {
			logv(v).Err(err).Trace("Error loading user for sync event")
			return
		}
		if err := s.publishSyncEventForUser(v, u); err != nil {
			logv(v).Err(err).Trace("Error publishing to user's sync topic")
		}
	}()
}

// publishSyncEventForUser publishes a sync message to the given user's sync topic, using v as
// the publishing visitor (for rate-limit accounting). No-op if the user has no sync topic.
func (s *Server) publishSyncEventForUser(v *visitor, u *user.User) error {
	if u == nil || u.SyncTopic == "" {
		return nil
	}
	logv(v).Field("sync_topic", u.SyncTopic).Trace("Publishing sync event to user's sync topic")
	syncTopic, err := s.topicFromID(nil, u.SyncTopic) // internal: no rate limit
	if err != nil {
		return err
	}
	messageBytes, err := json.Marshal(&apiAccountSyncTopicResponse{Event: syncTopicAccountSyncEvent})
	if err != nil {
		return err
	}
	m := model.NewDefaultMessage(syncTopic.ID, string(messageBytes))
	if err := s.dispatch(v, syncTopic, m, dispatchOpts{}); err != nil {
		return err
	}
	return nil
}
