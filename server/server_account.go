package server

import (
	"encoding/json"
	"errors"
	"github.com/stripe/stripe-go/v74"
	portalsession "github.com/stripe/stripe-go/v74/billingportal/session"
	"github.com/stripe/stripe-go/v74/checkout/session"
	"github.com/stripe/stripe-go/v74/subscription"
	"github.com/stripe/stripe-go/v74/webhook"
	"github.com/tidwall/gjson"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"net/http"
)

const (
	jsonBodyBytesLimit   = 4096
	stripeBodyBytesLimit = 16384
	subscriptionIDLength = 16
	createdByAPI         = "api"
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
		return errHTTPTooManyRequestsLimitAccountCreation
	}
	if err := s.userManager.AddUser(newAccount.Username, newAccount.Password, user.RoleUser, createdByAPI); err != nil { // TODO this should return a User
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	return nil
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
			Messages:                 limits.MessagesLimit,
			MessagesExpiryDuration:   int64(limits.MessagesExpiryDuration.Seconds()),
			Emails:                   limits.EmailsLimit,
			Reservations:             limits.ReservationsLimit,
			AttachmentTotalSize:      limits.AttachmentTotalSizeLimit,
			AttachmentFileSize:       limits.AttachmentFileSizeLimit,
			AttachmentExpiryDuration: int64(limits.AttachmentExpiryDuration.Seconds()),
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
		if v.user.Tier != nil {
			response.Tier = &apiAccountTier{
				Code: v.user.Tier.Code,
				Name: v.user.Tier.Name,
				Paid: v.user.Tier.Paid,
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
		} else if reservations >= v.user.Tier.ReservationsLimit {
			return errHTTPTooManyRequestsLimitReservations
		}
	}
	owner, username := v.user.Name, v.user.Name
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

func (s *Server) handleAccountReservationDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := accountReservationSingleRegex.FindStringSubmatch(r.URL.Path)
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

func (s *Server) handleAccountCheckoutSessionCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountTierChangeRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	}
	if tier.StripePriceID == "" {
		log.Info("Checkout: Downgrading to no tier")
		return errors.New("not a paid tier")
	} else if v.user.Billing != nil && v.user.Billing.StripeSubscriptionID != "" {
		log.Info("Checkout: Changing tier and subscription to %s", tier.Code)

		// Upgrade/downgrade tier
		sub, err := subscription.Get(v.user.Billing.StripeSubscriptionID, nil)
		if err != nil {
			return err
		}
		params := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(false),
			ProrationBehavior: stripe.String(string(stripe.SubscriptionSchedulePhaseProrationBehaviorCreateProrations)),
			Items: []*stripe.SubscriptionItemsParams{
				{
					ID:    stripe.String(sub.Items.Data[0].ID),
					Price: stripe.String(tier.StripePriceID),
				},
			},
		}
		_, err = subscription.Update(sub.ID, params)
		if err != nil {
			return err
		}
		response := &apiAccountCheckoutResponse{}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
		if err := json.NewEncoder(w).Encode(response); err != nil {
			return err
		}
		return nil
	} else {
		// Checkout flow
		log.Info("Checkout: No existing subscription, creating checkout flow")
	}

	successURL := s.config.BaseURL + accountCheckoutSuccessTemplate
	var stripeCustomerID *string
	if v.user.Billing != nil {
		stripeCustomerID = &v.user.Billing.StripeCustomerID
	}
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: &v.user.Name, // FIXME Should be user ID
		Customer:          stripeCustomerID,
		SuccessURL:        &successURL,
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(tier.StripePriceID),
				Quantity: stripe.Int64(1),
			},
		},
	}
	sess, err := session.New(params)
	if err != nil {
		return err
	}
	response := &apiAccountCheckoutResponse{
		RedirectURL: sess.URL,
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountCheckoutSessionSuccessGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// We don't have a v.user in this endpoint, only a userManager!
	matches := accountCheckoutSuccessRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	sessionID := matches[1]
	// FIXME how do I rate limit this?
	sess, err := session.Get(sessionID, nil)
	if err != nil {
		log.Warn("Stripe: %s", err)
		return errHTTPBadRequestInvalidStripeRequest
	} else if sess.Customer == nil || sess.Subscription == nil || sess.ClientReferenceID == "" {
		log.Warn("Stripe: Unexpected session, customer or subscription not found")
		return errHTTPBadRequestInvalidStripeRequest
	}
	sub, err := subscription.Get(sess.Subscription.ID, nil)
	if err != nil {
		return err
	} else if sub.Items == nil || len(sub.Items.Data) != 1 || sub.Items.Data[0].Price == nil {
		log.Error("Stripe: Unexpected subscription, expected exactly one line item")
		return errHTTPBadRequestInvalidStripeRequest
	}
	priceID := sub.Items.Data[0].Price.ID
	tier, err := s.userManager.TierByStripePrice(priceID)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(sess.ClientReferenceID)
	if err != nil {
		return err
	}
	if u.Billing == nil {
		u.Billing = &user.Billing{}
	}
	u.Billing.StripeCustomerID = sess.Customer.ID
	u.Billing.StripeSubscriptionID = sess.Subscription.ID
	if err := s.userManager.ChangeBilling(u); err != nil {
		return err
	}
	if err := s.userManager.ChangeTier(u.Name, tier.Code); err != nil {
		return err
	}
	accountURL := s.config.BaseURL + "/account" // FIXME
	http.Redirect(w, r, accountURL, http.StatusSeeOther)
	return nil
}

func (s *Server) handleAccountBillingPortalSessionCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user.Billing == nil {
		return errHTTPBadRequestNotAPaidUser
	}
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(v.user.Billing.StripeCustomerID),
		ReturnURL: stripe.String(s.config.BaseURL),
	}
	ps, err := portalsession.New(params)
	if err != nil {
		return err
	}
	response := &apiAccountBillingPortalRedirectResponse{
		RedirectURL: ps.URL,
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountBillingWebhookTrigger(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// We don't have a v.user in this endpoint, only a userManager!
	stripeSignature := r.Header.Get("Stripe-Signature")
	if stripeSignature == "" {
		return errHTTPBadRequestInvalidStripeRequest
	}
	body, err := util.Peek(r.Body, stripeBodyBytesLimit)
	if err != nil {
		return err
	} else if body.LimitReached {
		return errHTTPEntityTooLargeJSONBody
	}
	event, err := webhook.ConstructEvent(body.PeekedBytes, stripeSignature, s.config.StripeWebhookKey)
	if err != nil {
		log.Warn("Stripe: invalid request: %s", err.Error())
		return errHTTPBadRequestInvalidStripeRequest
	} else if event.Data == nil || event.Data.Raw == nil {
		log.Warn("Stripe: invalid request, data is nil")
		return errHTTPBadRequestInvalidStripeRequest
	}
	log.Info("Stripe: webhook event %s received", event.Type)
	stripeCustomerID := gjson.GetBytes(event.Data.Raw, "customer")
	if !stripeCustomerID.Exists() {
		return errHTTPBadRequestInvalidStripeRequest
	}
	switch event.Type {
	case "checkout.session.completed":
		// Payment is successful and the subscription is created.
		// Provision the subscription, save the customer ID.
		return s.handleAccountBillingWebhookCheckoutCompleted(stripeCustomerID.String(), event.Data.Raw)
	case "customer.subscription.updated":
		return s.handleAccountBillingWebhookSubscriptionUpdated(stripeCustomerID.String(), event.Data.Raw)
	case "invoice.paid":
		// Continue to provision the subscription as payments continue to be made.
		// Store the status in your database and check when a user accesses your service.
		// This approach helps you avoid hitting rate limits.
		return nil // FIXME
	case "invoice.payment_failed":
		// The payment failed or the customer does not have a valid payment method.
		// The subscription becomes past_due. Notify your customer and send them to the
		// customer portal to update their payment information.
		return nil // FIXME
	default:
		log.Warn("Stripe: unhandled webhook %s", event.Type)
		return nil
	}
}

func (s *Server) handleAccountBillingWebhookCheckoutCompleted(stripeCustomerID string, event json.RawMessage) error {
	log.Info("Stripe: checkout completed for customer %s", stripeCustomerID)
	return nil
}

func (s *Server) handleAccountBillingWebhookSubscriptionUpdated(stripeCustomerID string, event json.RawMessage) error {
	status := gjson.GetBytes(event, "status")
	priceID := gjson.GetBytes(event, "items.data.0.price.id")
	if !status.Exists() || !priceID.Exists() {
		return errHTTPBadRequestInvalidStripeRequest
	}
	log.Info("Stripe: customer %s: subscription updated to %s, with price %s", stripeCustomerID, status, priceID)
	u, err := s.userManager.UserByStripeCustomer(stripeCustomerID)
	if err != nil {
		return err
	}
	tier, err := s.userManager.TierByStripePrice(priceID.String())
	if err != nil {
		return err
	}
	if err := s.userManager.ChangeTier(u.Name, tier.Code); err != nil {
		return err
	}
	return nil
}
