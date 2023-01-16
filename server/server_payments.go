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
	"time"
)

const (
	stripeBodyBytesLimit = 16384
)

// handleAccountBillingSubscriptionChange facilitates all subscription/tier changes, including payment flows.
//
// FIXME this should be two functions!
//
// It handles two cases:
// - Create subscription: Transition from a user without Stripe subscription to a paid subscription (Checkout flow)
// - Change subscription: Switching between Stripe prices (& tiers) by changing the Stripe subscription
func (s *Server) handleAccountBillingSubscriptionChange(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccountTierChangeRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	}
	if v.user.Billing.StripeSubscriptionID == "" && tier.StripePriceID != "" {
		return s.handleAccountBillingSubscriptionAdd(w, v, tier)
	} else if v.user.Billing.StripeSubscriptionID != "" {
		return s.handleAccountBillingSubscriptionUpdate(w, v, tier)
	}
	return errors.New("invalid state")
}

// handleAccountBillingSubscriptionDelete facilitates downgrading a paid user to a tier-less user,
// and cancelling the Stripe subscription entirely
func (s *Server) handleAccountBillingSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user.Billing.StripeCustomerID == "" {
		return errHTTPBadRequestNotAPaidUser
	}
	if v.user.Billing.StripeSubscriptionID != "" {
		_, err := subscription.Cancel(v.user.Billing.StripeSubscriptionID, nil)
		if err != nil {
			return err
		}
	}
	if err := s.userManager.ResetTier(v.user.Name); err != nil {
		return err
	}
	v.user.Billing.StripeSubscriptionID = ""
	v.user.Billing.StripeSubscriptionStatus = ""
	v.user.Billing.StripeSubscriptionPaidUntil = time.Unix(0, 0)
	if err := s.userManager.ChangeBilling(v.user); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountBillingSubscriptionAdd(w http.ResponseWriter, v *visitor, tier *user.Tier) error {
	log.Info("Stripe: No existing subscription, creating checkout flow")
	var stripeCustomerID *string
	if v.user.Billing.StripeCustomerID != "" {
		stripeCustomerID = &v.user.Billing.StripeCustomerID
	}
	successURL := s.config.BaseURL + accountBillingSubscriptionCheckoutSuccessTemplate
	params := &stripe.CheckoutSessionParams{
		Customer:          stripeCustomerID, // A user may have previously deleted their subscription
		ClientReferenceID: &v.user.Name,     // FIXME Should be user ID
		SuccessURL:        &successURL,
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(tier.StripePriceID),
				Quantity: stripe.Int64(1),
			},
		},
		/*AutomaticTax: &stripe.CheckoutSessionAutomaticTaxParams{
			Enabled: stripe.Bool(true),
		},*/
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

func (s *Server) handleAccountBillingSubscriptionUpdate(w http.ResponseWriter, v *visitor, tier *user.Tier) error {
	log.Info("Stripe: Changing tier and subscription to %s", tier.Code)
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
}

func (s *Server) handleAccountCheckoutSessionSuccessGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// We don't have a v.user in this endpoint, only a userManager!
	matches := accountBillingSubscriptionCheckoutSuccessRegex.FindStringSubmatch(r.URL.Path)
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
	u.Billing.StripeCustomerID = sess.Customer.ID
	u.Billing.StripeSubscriptionID = sub.ID
	u.Billing.StripeSubscriptionStatus = sub.Status
	u.Billing.StripeSubscriptionPaidUntil = time.Unix(sub.CurrentPeriodEnd, 0)
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
	if v.user.Billing.StripeCustomerID == "" {
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

func (s *Server) handleAccountBillingWebhook(w http.ResponseWriter, r *http.Request, v *visitor) error {
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
		return errHTTPBadRequestInvalidStripeRequest
	} else if event.Data == nil || event.Data.Raw == nil {
		return errHTTPBadRequestInvalidStripeRequest
	}
	log.Info("Stripe: webhook event %s received", event.Type)
	stripeCustomerID := gjson.GetBytes(event.Data.Raw, "customer")
	if !stripeCustomerID.Exists() {
		return errHTTPBadRequestInvalidStripeRequest
	}
	switch event.Type {
	case "customer.subscription.updated":
		return s.handleAccountBillingWebhookSubscriptionUpdated(stripeCustomerID.String(), event.Data.Raw)
	case "customer.subscription.deleted":
		return s.handleAccountBillingWebhookSubscriptionDeleted(stripeCustomerID.String(), event.Data.Raw)
	default:
		return nil
	}
}

func (s *Server) handleAccountBillingWebhookSubscriptionUpdated(stripeCustomerID string, event json.RawMessage) error {
	status := gjson.GetBytes(event, "status")
	currentPeriodEnd := gjson.GetBytes(event, "current_period_end")
	priceID := gjson.GetBytes(event, "items.data.0.price.id")
	if !status.Exists() || !currentPeriodEnd.Exists() || !priceID.Exists() {
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
	u.Billing.StripeSubscriptionStatus = stripe.SubscriptionStatus(status.String())
	u.Billing.StripeSubscriptionPaidUntil = time.Unix(currentPeriodEnd.Int(), 0)
	if err := s.userManager.ChangeBilling(u); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccountBillingWebhookSubscriptionDeleted(stripeCustomerID string, event json.RawMessage) error {
	status := gjson.GetBytes(event, "status")
	if !status.Exists() {
		return errHTTPBadRequestInvalidStripeRequest
	}
	log.Info("Stripe: customer %s: subscription deleted, downgrading to unpaid tier", stripeCustomerID)
	u, err := s.userManager.UserByStripeCustomer(stripeCustomerID)
	if err != nil {
		return err
	}
	if err := s.userManager.ResetTier(u.Name); err != nil {
		return err
	}
	u.Billing.StripeSubscriptionID = ""
	u.Billing.StripeSubscriptionStatus = ""
	u.Billing.StripeSubscriptionPaidUntil = time.Unix(0, 0)
	if err := s.userManager.ChangeBilling(u); err != nil {
		return err
	}
	return nil
}
