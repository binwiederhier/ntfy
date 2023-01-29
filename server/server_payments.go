package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stripe/stripe-go/v74"
	portalsession "github.com/stripe/stripe-go/v74/billingportal/session"
	"github.com/stripe/stripe-go/v74/checkout/session"
	"github.com/stripe/stripe-go/v74/customer"
	"github.com/stripe/stripe-go/v74/price"
	"github.com/stripe/stripe-go/v74/subscription"
	"github.com/stripe/stripe-go/v74/webhook"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"io"
	"net/http"
	"net/netip"
	"time"
)

// Payments in ntfy are done via Stripe.
//
// Pretty much all payments related things are in this file. The following processes
// handle payments:
//
// - Checkout:
//      Creating a Stripe customer and subscription via the Checkout flow. This flow is only used if the
//      ntfy user is not already a Stripe customer. This requires redirecting to the Stripe checkout page.
//      It is implemented in handleAccountBillingSubscriptionCreate and the success callback
//      handleAccountBillingSubscriptionCreateSuccess.
// - Update subscription:
//      Switching between Stripe subscriptions (upgrade/downgrade) is handled via
//      handleAccountBillingSubscriptionUpdate. This also handles proration.
// - Cancel subscription (at period end):
//      Users can cancel the Stripe subscription via the web app at the end of the billing period. This
//      simply updates the subscription and Stripe will cancel it. Users cannot immediately cancel the
//      subscription.
// - Webhooks:
//      Whenever a subscription changes (updated, deleted), Stripe sends us a request via a webhook.
//      This is used to keep the local user database fields up to date. Stripe is the source of truth.
//      What Stripe says is mirrored and not questioned.

var (
	errNotAPaidTier                 = errors.New("tier does not have billing price identifier")
	errMultipleBillingSubscriptions = errors.New("cannot have multiple billing subscriptions")
	errNoBillingSubscription        = errors.New("user does not have an active billing subscription")
)

var (
	retryUserDelays = []time.Duration{3 * time.Second, 5 * time.Second, 7 * time.Second}
)

// handleBillingTiersGet returns all available paid tiers, and the free tier. This is to populate the upgrade dialog
// in the UI. Note that this endpoint does NOT have a user context (no u!).
func (s *Server) handleBillingTiersGet(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	tiers, err := s.userManager.Tiers()
	if err != nil {
		return err
	}
	freeTier := configBasedVisitorLimits(s.config)
	response := []*apiAccountBillingTier{
		{
			// This is a bit of a hack: This is the "Free" tier. It has no tier code, name or price.
			Limits: &apiAccountLimits{
				Basis:                    string(visitorLimitBasisIP),
				Messages:                 freeTier.MessageLimit,
				MessagesExpiryDuration:   int64(freeTier.MessageExpiryDuration.Seconds()),
				Emails:                   freeTier.EmailLimit,
				Reservations:             freeTier.ReservationsLimit,
				AttachmentTotalSize:      freeTier.AttachmentTotalSizeLimit,
				AttachmentFileSize:       freeTier.AttachmentFileSizeLimit,
				AttachmentExpiryDuration: int64(freeTier.AttachmentExpiryDuration.Seconds()),
			},
		},
	}
	prices, err := s.priceCache.Value()
	if err != nil {
		return err
	}
	for _, tier := range tiers {
		priceStr, ok := prices[tier.StripePriceID]
		if tier.StripePriceID == "" || !ok {
			continue
		}
		response = append(response, &apiAccountBillingTier{
			Code:  tier.Code,
			Name:  tier.Name,
			Price: priceStr,
			Limits: &apiAccountLimits{
				Basis:                    string(visitorLimitBasisTier),
				Messages:                 tier.MessageLimit,
				MessagesExpiryDuration:   int64(tier.MessageExpiryDuration.Seconds()),
				Emails:                   tier.EmailLimit,
				Reservations:             tier.ReservationLimit,
				AttachmentTotalSize:      tier.AttachmentTotalSizeLimit,
				AttachmentFileSize:       tier.AttachmentFileSizeLimit,
				AttachmentExpiryDuration: int64(tier.AttachmentExpiryDuration.Seconds()),
			},
		})
	}
	return s.writeJSON(w, response)
}

// handleAccountBillingSubscriptionCreate creates a Stripe checkout flow to create a user subscription. The tier
// will be updated by a subsequent webhook from Stripe, once the subscription becomes active.
func (s *Server) handleAccountBillingSubscriptionCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	if u.Billing.StripeSubscriptionID != "" {
		return errHTTPBadRequestBillingSubscriptionExists
	}
	req, err := readJSONWithLimit[apiAccountBillingSubscriptionChangeRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	} else if tier.StripePriceID == "" {
		return errNotAPaidTier
	}
	log.Info("%s Creating Stripe checkout flow", logHTTPPrefix(v, r))
	var stripeCustomerID *string
	if u.Billing.StripeCustomerID != "" {
		stripeCustomerID = &u.Billing.StripeCustomerID
		stripeCustomer, err := s.stripe.GetCustomer(u.Billing.StripeCustomerID)
		if err != nil {
			return err
		} else if stripeCustomer.Subscriptions != nil && len(stripeCustomer.Subscriptions.Data) > 0 {
			return errMultipleBillingSubscriptions
		}
	}
	successURL := s.config.BaseURL + apiAccountBillingSubscriptionCheckoutSuccessTemplate
	params := &stripe.CheckoutSessionParams{
		Customer:            stripeCustomerID, // A user may have previously deleted their subscription
		ClientReferenceID:   &u.ID,
		SuccessURL:          &successURL,
		Mode:                stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		AllowPromotionCodes: stripe.Bool(true),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(tier.StripePriceID),
				Quantity: stripe.Int64(1),
			},
		},
		Params: stripe.Params{
			Metadata: map[string]string{
				"user_id": u.ID,
			},
		},
	}
	sess, err := s.stripe.NewCheckoutSession(params)
	if err != nil {
		return err
	}
	response := &apiAccountBillingSubscriptionCreateResponse{
		RedirectURL: sess.URL,
	}
	return s.writeJSON(w, response)
}

// handleAccountBillingSubscriptionCreateSuccess is called after the Stripe checkout session has succeeded. We use
// the session ID in the URL to retrieve the Stripe subscription and update the local database. This is the first
// and only time we can map the local username with the Stripe customer ID.
func (s *Server) handleAccountBillingSubscriptionCreateSuccess(w http.ResponseWriter, r *http.Request, v *visitor) error {
	// We don't have v.User() in this endpoint, only a userManager!
	matches := apiAccountBillingSubscriptionCheckoutSuccessRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	sessionID := matches[1]
	sess, err := s.stripe.GetSession(sessionID) // FIXME How do we rate limit this?
	if err != nil {
		return err
	} else if sess.Customer == nil || sess.Subscription == nil || sess.ClientReferenceID == "" {
		return wrapErrHTTP(errHTTPBadRequestBillingRequestInvalid, "customer or subscription not found")
	}
	sub, err := s.stripe.GetSubscription(sess.Subscription.ID)
	if err != nil {
		return err
	} else if sub.Items == nil || len(sub.Items.Data) != 1 || sub.Items.Data[0].Price == nil {
		return wrapErrHTTP(errHTTPBadRequestBillingRequestInvalid, "more than one line item in existing subscription")
	}
	tier, err := s.userManager.TierByStripePrice(sub.Items.Data[0].Price.ID)
	if err != nil {
		return err
	}
	u, err := s.userManager.UserByID(sess.ClientReferenceID)
	if err != nil {
		return err
	}
	v.SetUser(u)
	customerParams := &stripe.CustomerParams{
		Params: stripe.Params{
			Metadata: map[string]string{
				"user_id":   u.ID,
				"user_name": u.Name,
			},
		},
	}
	if _, err := s.stripe.UpdateCustomer(sess.Customer.ID, customerParams); err != nil {
		return err
	}
	if err := s.updateSubscriptionAndTier(logHTTPPrefix(v, r), u, tier, sess.Customer.ID, sub.ID, string(sub.Status), sub.CurrentPeriodEnd, sub.CancelAt); err != nil {
		return err
	}
	http.Redirect(w, r, s.config.BaseURL+accountPath, http.StatusSeeOther)
	return nil
}

// handleAccountBillingSubscriptionUpdate updates an existing Stripe subscription to a new price, and updates
// a user's tier accordingly. This endpoint only works if there is an existing subscription.
func (s *Server) handleAccountBillingSubscriptionUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	if u.Billing.StripeSubscriptionID == "" {
		return errNoBillingSubscription
	}
	req, err := readJSONWithLimit[apiAccountBillingSubscriptionChangeRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	}
	log.Info("%s Changing billing tier to %s (price %s) for subscription %s", logHTTPPrefix(v, r), tier.Code, tier.StripePriceID, u.Billing.StripeSubscriptionID)
	sub, err := s.stripe.GetSubscription(u.Billing.StripeSubscriptionID)
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
	_, err = s.stripe.UpdateSubscription(sub.ID, params)
	if err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountBillingSubscriptionDelete facilitates downgrading a paid user to a tier-less user,
// and cancelling the Stripe subscription entirely
func (s *Server) handleAccountBillingSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	log.Info("%s Deleting billing subscription %s", logHTTPPrefix(v, r), u.Billing.StripeSubscriptionID)
	if u.Billing.StripeSubscriptionID != "" {
		params := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(true),
		}
		_, err := s.stripe.UpdateSubscription(u.Billing.StripeSubscriptionID, params)
		if err != nil {
			return err
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountBillingPortalSessionCreate creates a session to the customer billing portal, and returns the
// redirect URL. The billing portal allows customers to change their payment methods, and cancel the subscription.
func (s *Server) handleAccountBillingPortalSessionCreate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	if u.Billing.StripeCustomerID == "" {
		return errHTTPBadRequestNotAPaidUser
	}
	log.Info("%s Creating billing portal session", logHTTPPrefix(v, r))
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(u.Billing.StripeCustomerID),
		ReturnURL: stripe.String(s.config.BaseURL),
	}
	ps, err := s.stripe.NewPortalSession(params)
	if err != nil {
		return err
	}
	response := &apiAccountBillingPortalRedirectResponse{
		RedirectURL: ps.URL,
	}
	return s.writeJSON(w, response)
}

// handleAccountBillingWebhook handles incoming Stripe webhooks. It mainly keeps the local user database in sync
// with the Stripe view of the world. This endpoint is authorized via the Stripe webhook secret. Note that the
// visitor (v) in this endpoint is the Stripe API, so we don't have u available.
func (s *Server) handleAccountBillingWebhook(_ http.ResponseWriter, r *http.Request, _ *visitor) error {
	stripeSignature := r.Header.Get("Stripe-Signature")
	if stripeSignature == "" {
		return errHTTPBadRequestBillingRequestInvalid
	}
	body, err := util.Peek(r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	} else if body.LimitReached {
		return errHTTPEntityTooLargeJSONBody
	}
	event, err := s.stripe.ConstructWebhookEvent(body.PeekedBytes, stripeSignature, s.config.StripeWebhookKey)
	if err != nil {
		return err
	} else if event.Data == nil || event.Data.Raw == nil {
		return errHTTPBadRequestBillingRequestInvalid
	}
	switch event.Type {
	case "customer.subscription.updated":
		return s.handleAccountBillingWebhookSubscriptionUpdated(event.Data.Raw)
	case "customer.subscription.deleted":
		return s.handleAccountBillingWebhookSubscriptionDeleted(event.Data.Raw)
	default:
		log.Warn("STRIPE Unhandled webhook event %s received", event.Type)
		return nil
	}
}

func (s *Server) handleAccountBillingWebhookSubscriptionUpdated(event json.RawMessage) error {
	ev, err := util.UnmarshalJSON[apiStripeSubscriptionUpdatedEvent](io.NopCloser(bytes.NewReader(event)))
	if err != nil {
		return err
	} else if ev.ID == "" || ev.Customer == "" || ev.Status == "" || ev.CurrentPeriodEnd == 0 || ev.Items == nil || len(ev.Items.Data) != 1 || ev.Items.Data[0].Price == nil || ev.Items.Data[0].Price.ID == "" {
		return errHTTPBadRequestBillingRequestInvalid
	}
	subscriptionID, priceID := ev.ID, ev.Items.Data[0].Price.ID
	log.Info("%s Updating subscription to status %s, with price %s", logStripePrefix(ev.Customer, ev.ID), ev.Status, priceID)
	userFn := func() (*user.User, error) {
		return s.userManager.UserByStripeCustomer(ev.Customer)
	}
	u, err := util.Retry[user.User](userFn, retryUserDelays...)
	if err != nil {
		return err
	}
	tier, err := s.userManager.TierByStripePrice(priceID)
	if err != nil {
		return err
	}
	if err := s.updateSubscriptionAndTier(logStripePrefix(ev.Customer, ev.ID), u, tier, ev.Customer, subscriptionID, ev.Status, ev.CurrentPeriodEnd, ev.CancelAt); err != nil {
		return err
	}
	s.publishSyncEventAsync(s.visitor(netip.IPv4Unspecified(), u))
	return nil
}

func (s *Server) handleAccountBillingWebhookSubscriptionDeleted(event json.RawMessage) error {
	ev, err := util.UnmarshalJSON[apiStripeSubscriptionDeletedEvent](io.NopCloser(bytes.NewReader(event)))
	if err != nil {
		return err
	} else if ev.Customer == "" {
		return errHTTPBadRequestBillingRequestInvalid
	}
	log.Info("%s Subscription deleted, downgrading to unpaid tier", logStripePrefix(ev.Customer, ev.ID))
	u, err := s.userManager.UserByStripeCustomer(ev.Customer)
	if err != nil {
		return err
	}
	if err := s.updateSubscriptionAndTier(logStripePrefix(ev.Customer, ev.ID), u, nil, ev.Customer, "", "", 0, 0); err != nil {
		return err
	}
	s.publishSyncEventAsync(s.visitor(netip.IPv4Unspecified(), u))
	return nil
}

func (s *Server) updateSubscriptionAndTier(logPrefix string, u *user.User, tier *user.Tier, customerID, subscriptionID, status string, paidUntil, cancelAt int64) error {
	reservationsLimit := visitorDefaultReservationsLimit
	if tier != nil {
		reservationsLimit = tier.ReservationLimit
	}
	if err := s.maybeRemoveMessagesAndExcessReservations(logPrefix, u, reservationsLimit); err != nil {
		return err
	}
	if tier == nil {
		if err := s.userManager.ResetTier(u.Name); err != nil {
			return err
		}
	} else {
		if err := s.userManager.ChangeTier(u.Name, tier.Code); err != nil {
			return err
		}
	}
	// Update billing fields
	billing := &user.Billing{
		StripeCustomerID:            customerID,
		StripeSubscriptionID:        subscriptionID,
		StripeSubscriptionStatus:    stripe.SubscriptionStatus(status),
		StripeSubscriptionPaidUntil: time.Unix(paidUntil, 0),
		StripeSubscriptionCancelAt:  time.Unix(cancelAt, 0),
	}
	if err := s.userManager.ChangeBilling(u.Name, billing); err != nil {
		return err
	}
	return nil
}

// fetchStripePrices contacts the Stripe API to retrieve all prices. This is used by the server to cache the prices
// in memory, and ultimately for the web app to display the price table.
func (s *Server) fetchStripePrices() (map[string]string, error) {
	log.Debug("Caching prices from Stripe API")
	priceMap := make(map[string]string)
	prices, err := s.stripe.ListPrices(&stripe.PriceListParams{Active: stripe.Bool(true)})
	if err != nil {
		log.Warn("Fetching Stripe prices failed: %s", err.Error())
		return nil, err
	}
	for _, p := range prices {
		if p.UnitAmount%100 == 0 {
			priceMap[p.ID] = fmt.Sprintf("$%d", p.UnitAmount/100)
		} else {
			priceMap[p.ID] = fmt.Sprintf("$%.2f", float64(p.UnitAmount)/100)
		}
		log.Trace("- Caching price %s = %v", p.ID, priceMap[p.ID])
	}
	return priceMap, nil
}

// stripeAPI is a small interface to facilitate mocking of the Stripe API
type stripeAPI interface {
	NewCheckoutSession(params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error)
	NewPortalSession(params *stripe.BillingPortalSessionParams) (*stripe.BillingPortalSession, error)
	ListPrices(params *stripe.PriceListParams) ([]*stripe.Price, error)
	GetCustomer(id string) (*stripe.Customer, error)
	GetSession(id string) (*stripe.CheckoutSession, error)
	GetSubscription(id string) (*stripe.Subscription, error)
	UpdateCustomer(id string, params *stripe.CustomerParams) (*stripe.Customer, error)
	UpdateSubscription(id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error)
	CancelSubscription(id string) (*stripe.Subscription, error)
	ConstructWebhookEvent(payload []byte, header string, secret string) (stripe.Event, error)
}

// realStripeAPI is a thin shim around the Stripe functions to facilitate mocking
type realStripeAPI struct{}

var _ stripeAPI = (*realStripeAPI)(nil)

func newStripeAPI() stripeAPI {
	return &realStripeAPI{}
}

func (s *realStripeAPI) NewCheckoutSession(params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
	return session.New(params)
}

func (s *realStripeAPI) NewPortalSession(params *stripe.BillingPortalSessionParams) (*stripe.BillingPortalSession, error) {
	return portalsession.New(params)
}

func (s *realStripeAPI) ListPrices(params *stripe.PriceListParams) ([]*stripe.Price, error) {
	prices := make([]*stripe.Price, 0)
	iter := price.List(params)
	for iter.Next() {
		prices = append(prices, iter.Price())
	}
	if iter.Err() != nil {
		return nil, iter.Err()
	}
	return prices, nil
}

func (s *realStripeAPI) GetCustomer(id string) (*stripe.Customer, error) {
	return customer.Get(id, nil)
}

func (s *realStripeAPI) GetSession(id string) (*stripe.CheckoutSession, error) {
	return session.Get(id, nil)
}

func (s *realStripeAPI) GetSubscription(id string) (*stripe.Subscription, error) {
	return subscription.Get(id, nil)
}

func (s *realStripeAPI) UpdateCustomer(id string, params *stripe.CustomerParams) (*stripe.Customer, error) {
	return customer.Update(id, params)
}

func (s *realStripeAPI) UpdateSubscription(id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
	return subscription.Update(id, params)
}

func (s *realStripeAPI) CancelSubscription(id string) (*stripe.Subscription, error) {
	return subscription.Cancel(id, nil)
}

func (s *realStripeAPI) ConstructWebhookEvent(payload []byte, header string, secret string) (stripe.Event, error) {
	return webhook.ConstructEvent(payload, header, secret)
}
