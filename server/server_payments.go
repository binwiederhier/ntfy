package server

import (
	"encoding/json"
	"errors"
	"github.com/stripe/stripe-go/v74"
	portalsession "github.com/stripe/stripe-go/v74/billingportal/session"
	"github.com/stripe/stripe-go/v74/checkout/session"
	"github.com/stripe/stripe-go/v74/customer"
	"github.com/stripe/stripe-go/v74/subscription"
	"github.com/stripe/stripe-go/v74/webhook"
	"github.com/tidwall/gjson"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"net/http"
	"net/netip"
	"time"
)

const (
	stripeBodyBytesLimit = 16384
)

// handleAccountBillingSubscriptionCreate creates a Stripe checkout flow to create a user subscription. The tier
// will be updated by a subsequent webhook from Stripe, once the subscription becomes active.
func (s *Server) handleAccountBillingSubscriptionCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user.Billing.StripeSubscriptionID != "" {
		return errors.New("subscription already exists") //FIXME
	}
	req, err := readJSONWithLimit[apiAccountBillingSubscriptionChangeRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	}
	if tier.StripePriceID == "" {
		return errors.New("invalid tier") //FIXME
	}
	log.Info("Stripe: No existing subscription, creating checkout flow")
	var stripeCustomerID *string
	if v.user.Billing.StripeCustomerID != "" {
		stripeCustomerID = &v.user.Billing.StripeCustomerID
		stripeCustomer, err := customer.Get(v.user.Billing.StripeCustomerID, nil)
		if err != nil {
			return err
		} else if stripeCustomer.Subscriptions != nil && len(stripeCustomer.Subscriptions.Data) > 0 {
			return errors.New("customer cannot have more than one subscription") //FIXME
		}
	}
	successURL := s.config.BaseURL + "/account" //+ accountBillingSubscriptionCheckoutSuccessTemplate
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

func (s *Server) handleAccountBillingSubscriptionCreateSuccess(w http.ResponseWriter, r *http.Request, _ *visitor) error {
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
	u.Billing.StripeSubscriptionCancelAt = time.Unix(sub.CancelAt, 0)
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

// handleAccountBillingSubscriptionUpdate updates an existing Stripe subscription to a new price, and updates
// a user's tier accordingly. This endpoint only works if there is an existing subscription.
func (s *Server) handleAccountBillingSubscriptionUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user.Billing.StripeSubscriptionID != "" {
		return errors.New("no existing subscription for user")
	}
	req, err := readJSONWithLimit[apiAccountBillingSubscriptionChangeRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	}
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
	response := &apiAccountCheckoutResponse{} // FIXME
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // FIXME remove this
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

// handleAccountBillingSubscriptionDelete facilitates downgrading a paid user to a tier-less user,
// and cancelling the Stripe subscription entirely
func (s *Server) handleAccountBillingSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user.Billing.StripeCustomerID == "" {
		return errHTTPBadRequestNotAPaidUser
	}
	if v.user.Billing.StripeSubscriptionID != "" {
		params := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(true),
		}
		_, err := subscription.Update(v.user.Billing.StripeSubscriptionID, params)
		if err != nil {
			return err
		}
	}
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

func (s *Server) handleAccountBillingWebhook(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	// Note that the visitor (v) in this endpoint is the Stripe API, so we don't have v.user available
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
	switch event.Type {
	case "customer.subscription.updated":
		return s.handleAccountBillingWebhookSubscriptionUpdated(event.Data.Raw)
	case "customer.subscription.deleted":
		return s.handleAccountBillingWebhookSubscriptionDeleted(event.Data.Raw)
	default:
		return nil
	}
}

func (s *Server) handleAccountBillingWebhookSubscriptionUpdated(event json.RawMessage) error {
	subscriptionID := gjson.GetBytes(event, "id")
	customerID := gjson.GetBytes(event, "customer")
	status := gjson.GetBytes(event, "status")
	currentPeriodEnd := gjson.GetBytes(event, "current_period_end")
	cancelAt := gjson.GetBytes(event, "cancel_at")
	priceID := gjson.GetBytes(event, "items.data.0.price.id")
	if !subscriptionID.Exists() || !status.Exists() || !currentPeriodEnd.Exists() || !cancelAt.Exists() || !priceID.Exists() {
		return errHTTPBadRequestInvalidStripeRequest
	}
	u, err := s.userManager.UserByStripeCustomer(customerID.String())
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
	u.Billing.StripeSubscriptionID = subscriptionID.String()
	u.Billing.StripeSubscriptionStatus = stripe.SubscriptionStatus(status.String())
	u.Billing.StripeSubscriptionPaidUntil = time.Unix(currentPeriodEnd.Int(), 0)
	u.Billing.StripeSubscriptionCancelAt = time.Unix(cancelAt.Int(), 0)
	if err := s.userManager.ChangeBilling(u); err != nil {
		return err
	}
	log.Info("Stripe: customer %s: subscription updated to %s, with price %s", customerID.String(), status, priceID)
	s.publishSyncEventAsync(s.visitorFromUser(u, netip.IPv4Unspecified()))
	return nil
}

func (s *Server) handleAccountBillingWebhookSubscriptionDeleted(event json.RawMessage) error {
	stripeCustomerID := gjson.GetBytes(event, "customer")
	if !stripeCustomerID.Exists() {
		return errHTTPBadRequestInvalidStripeRequest
	}
	log.Info("Stripe: customer %s: subscription deleted, downgrading to unpaid tier", stripeCustomerID.String())
	u, err := s.userManager.UserByStripeCustomer(stripeCustomerID.String())
	if err != nil {
		return err
	}
	if err := s.userManager.ResetTier(u.Name); err != nil {
		return err
	}
	u.Billing.StripeSubscriptionID = ""
	u.Billing.StripeSubscriptionStatus = ""
	u.Billing.StripeSubscriptionPaidUntil = time.Unix(0, 0)
	u.Billing.StripeSubscriptionCancelAt = time.Unix(0, 0)
	if err := s.userManager.ChangeBilling(u); err != nil {
		return err
	}
	s.publishSyncEventAsync(s.visitorFromUser(u, netip.IPv4Unspecified()))
	return nil
}
