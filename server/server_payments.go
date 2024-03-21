package server

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/stripe/stripe-go/v74"
	portalsession "github.com/stripe/stripe-go/v74/billingportal/session"
	"github.com/stripe/stripe-go/v74/checkout/session"
	"github.com/stripe/stripe-go/v74/customer"
	"github.com/stripe/stripe-go/v74/price"
	"github.com/stripe/stripe-go/v74/subscription"
	"github.com/stripe/stripe-go/v74/webhook"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
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
				Calls:                    freeTier.CallLimit,
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
		priceMonth, priceYear := prices[tier.StripeMonthlyPriceID], prices[tier.StripeYearlyPriceID]
		if priceMonth == 0 || priceYear == 0 { // Only allow tiers that have both prices!
			continue
		}
		response = append(response, &apiAccountBillingTier{
			Code: tier.Code,
			Name: tier.Name,
			Prices: &apiAccountBillingPrices{
				Month: priceMonth,
				Year:  priceYear,
			},
			Limits: &apiAccountLimits{
				Basis:                    string(visitorLimitBasisTier),
				Messages:                 tier.MessageLimit,
				MessagesExpiryDuration:   int64(tier.MessageExpiryDuration.Seconds()),
				Emails:                   tier.EmailLimit,
				Calls:                    tier.CallLimit,
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
	req, err := readJSONWithLimit[apiAccountBillingSubscriptionChangeRequest](r.Body, httpBodyBytesLimit, false)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	}
	var priceID string
	if req.Interval == string(stripe.PriceRecurringIntervalMonth) && tier.StripeMonthlyPriceID != "" {
		priceID = tier.StripeMonthlyPriceID
	} else if req.Interval == string(stripe.PriceRecurringIntervalYear) && tier.StripeYearlyPriceID != "" {
		priceID = tier.StripeYearlyPriceID
	} else {
		return errNotAPaidTier
	}
	logvr(v, r).
		With(tier).
		Fields(log.Context{
			"stripe_price_id":              priceID,
			"stripe_subscription_interval": req.Interval,
		}).
		Tag(tagStripe).
		Info("Creating Stripe checkout flow")
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
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		AutomaticTax: &stripe.CheckoutSessionAutomaticTaxParams{
			Enabled: stripe.Bool(true),
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
		return errHTTPBadRequestBillingRequestInvalid.Wrap("customer or subscription not found")
	}
	sub, err := s.stripe.GetSubscription(sess.Subscription.ID)
	if err != nil {
		return err
	} else if sub.Items == nil || len(sub.Items.Data) != 1 || sub.Items.Data[0].Price == nil || sub.Items.Data[0].Price.Recurring == nil {
		return errHTTPBadRequestBillingRequestInvalid.Wrap("more than one line item in existing subscription")
	}
	priceID, interval := sub.Items.Data[0].Price.ID, sub.Items.Data[0].Price.Recurring.Interval
	tier, err := s.userManager.TierByStripePrice(priceID)
	if err != nil {
		return err
	}
	u, err := s.userManager.UserByID(sess.ClientReferenceID)
	if err != nil {
		return err
	}
	v.SetUser(u)
	logvr(v, r).
		With(tier).
		Tag(tagStripe).
		Fields(log.Context{
			"stripe_customer_id":             sess.Customer.ID,
			"stripe_price_id":                priceID,
			"stripe_subscription_id":         sub.ID,
			"stripe_subscription_status":     string(sub.Status),
			"stripe_subscription_interval":   string(interval),
			"stripe_subscription_paid_until": sub.CurrentPeriodEnd,
		}).
		Info("Stripe checkout flow succeeded, updating user tier and subscription")
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
	if err := s.updateSubscriptionAndTier(r, v, u, tier, sess.Customer.ID, sub.ID, string(sub.Status), string(interval), sub.CurrentPeriodEnd, sub.CancelAt); err != nil {
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
	req, err := readJSONWithLimit[apiAccountBillingSubscriptionChangeRequest](r.Body, httpBodyBytesLimit, false)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	}
	var priceID string
	if req.Interval == string(stripe.PriceRecurringIntervalMonth) && tier.StripeMonthlyPriceID != "" {
		priceID = tier.StripeMonthlyPriceID
	} else if req.Interval == string(stripe.PriceRecurringIntervalYear) && tier.StripeYearlyPriceID != "" {
		priceID = tier.StripeYearlyPriceID
	} else {
		return errNotAPaidTier
	}
	logvr(v, r).
		Tag(tagStripe).
		Fields(log.Context{
			"new_tier_id":                           tier.ID,
			"new_tier_code":                         tier.Code,
			"new_tier_stripe_price_id":              priceID,
			"new_tier_stripe_subscription_interval": req.Interval,
			// Other stripe_* fields filled by visitor context
		}).
		Info("Changing Stripe subscription and billing tier to %s/%s (price %s, %s)", tier.ID, tier.Name, priceID, req.Interval)
	sub, err := s.stripe.GetSubscription(u.Billing.StripeSubscriptionID)
	if err != nil {
		return err
	} else if sub.Items == nil || len(sub.Items.Data) != 1 {
		return errHTTPBadRequestBillingRequestInvalid.Wrap("no items, or more than one item")
	}
	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(false),
		ProrationBehavior: stripe.String(string(stripe.SubscriptionSchedulePhaseProrationBehaviorAlwaysInvoice)),
		Items: []*stripe.SubscriptionItemsParams{
			{
				ID:    stripe.String(sub.Items.Data[0].ID),
				Price: stripe.String(priceID),
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
// and cancelling the Stripe subscription entirely. Note that this does not actually change the tier.
// That is done by a webhook at the period end (in X days).
func (s *Server) handleAccountBillingSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	logvr(v, r).Tag(tagStripe).Info("Deleting Stripe subscription")
	u := v.User()
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
	logvr(v, r).Tag(tagStripe).Info("Creating Stripe billing portal session")
	u := v.User()
	if u.Billing.StripeCustomerID == "" {
		return errHTTPBadRequestNotAPaidUser
	}
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
func (s *Server) handleAccountBillingWebhook(_ http.ResponseWriter, r *http.Request, v *visitor) error {
	stripeSignature := r.Header.Get("Stripe-Signature")
	if stripeSignature == "" {
		return errHTTPBadRequestBillingRequestInvalid
	}
	body, err := util.Peek(r.Body, httpBodyBytesLimit)
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
		return s.handleAccountBillingWebhookSubscriptionUpdated(r, v, event)
	case "customer.subscription.deleted":
		return s.handleAccountBillingWebhookSubscriptionDeleted(r, v, event)
	default:
		logvr(v, r).
			Tag(tagStripe).
			Field("stripe_webhook_type", event.Type).
			Warn("Unhandled Stripe webhook event %s received", event.Type)
		return nil
	}
}

func (s *Server) handleAccountBillingWebhookSubscriptionUpdated(r *http.Request, v *visitor, event stripe.Event) error {
	ev, err := util.UnmarshalJSON[apiStripeSubscriptionUpdatedEvent](io.NopCloser(bytes.NewReader(event.Data.Raw)))
	if err != nil {
		return err
	} else if ev.ID == "" || ev.Customer == "" || ev.Status == "" || ev.CurrentPeriodEnd == 0 || ev.Items == nil || len(ev.Items.Data) != 1 || ev.Items.Data[0].Price == nil || ev.Items.Data[0].Price.ID == "" || ev.Items.Data[0].Price.Recurring == nil {
		logvr(v, r).Tag(tagStripe).Field("stripe_request", fmt.Sprintf("%#v", ev)).Warn("Unexpected request from Stripe")
		return errHTTPBadRequestBillingRequestInvalid
	}
	subscriptionID, priceID, interval := ev.ID, ev.Items.Data[0].Price.ID, ev.Items.Data[0].Price.Recurring.Interval
	logvr(v, r).
		Tag(tagStripe).
		Fields(log.Context{
			"stripe_webhook_type":            event.Type,
			"stripe_customer_id":             ev.Customer,
			"stripe_price_id":                priceID,
			"stripe_subscription_id":         ev.ID,
			"stripe_subscription_status":     ev.Status,
			"stripe_subscription_interval":   interval,
			"stripe_subscription_paid_until": ev.CurrentPeriodEnd,
			"stripe_subscription_cancel_at":  ev.CancelAt,
		}).
		Info("Updating subscription to status %s, with price %s", ev.Status, priceID)
	userFn := func() (*user.User, error) {
		return s.userManager.UserByStripeCustomer(ev.Customer)
	}
	// We retry the user retrieval function, because during the Stripe checkout, there a race between the browser
	// checkout success redirect (see handleAccountBillingSubscriptionCreateSuccess), and this webhook. The checkout
	// success call is the one that updates the user with the Stripe customer ID.
	u, err := util.Retry[user.User](userFn, retryUserDelays...)
	if err != nil {
		return err
	}
	v.SetUser(u)
	tier, err := s.userManager.TierByStripePrice(priceID)
	if err != nil {
		return err
	}
	if err := s.updateSubscriptionAndTier(r, v, u, tier, ev.Customer, subscriptionID, ev.Status, string(interval), ev.CurrentPeriodEnd, ev.CancelAt); err != nil {
		return err
	}
	s.publishSyncEventAsync(s.visitor(netip.IPv4Unspecified(), u))
	return nil
}

func (s *Server) handleAccountBillingWebhookSubscriptionDeleted(r *http.Request, v *visitor, event stripe.Event) error {
	ev, err := util.UnmarshalJSON[apiStripeSubscriptionDeletedEvent](io.NopCloser(bytes.NewReader(event.Data.Raw)))
	if err != nil {
		return err
	} else if ev.Customer == "" {
		return errHTTPBadRequestBillingRequestInvalid
	}
	u, err := s.userManager.UserByStripeCustomer(ev.Customer)
	if err != nil {
		return err
	}
	v.SetUser(u)
	logvr(v, r).
		Tag(tagStripe).
		Field("stripe_webhook_type", event.Type).
		Info("Subscription deleted, downgrading to unpaid tier")
	if err := s.updateSubscriptionAndTier(r, v, u, nil, ev.Customer, "", "", "", 0, 0); err != nil {
		return err
	}
	s.publishSyncEventAsync(s.visitor(netip.IPv4Unspecified(), u))
	return nil
}

func (s *Server) updateSubscriptionAndTier(r *http.Request, v *visitor, u *user.User, tier *user.Tier, customerID, subscriptionID, status, interval string, paidUntil, cancelAt int64) error {
	reservationsLimit := visitorDefaultReservationsLimit
	if tier != nil {
		reservationsLimit = tier.ReservationLimit
	}
	if err := s.maybeRemoveMessagesAndExcessReservations(r, v, u, reservationsLimit); err != nil {
		return err
	}
	if tier == nil && u.Tier != nil {
		logvr(v, r).Tag(tagStripe).Info("Resetting tier for user %s", u.Name)
		if err := s.userManager.ResetTier(u.Name); err != nil {
			return err
		}
	} else if tier != nil && u.TierID() != tier.ID {
		logvr(v, r).
			Tag(tagStripe).
			Fields(log.Context{
				"new_tier_id":   tier.ID,
				"new_tier_code": tier.Code,
			}).
			Info("Changing tier to tier %s (%s) for user %s", tier.ID, tier.Name, u.Name)
		if err := s.userManager.ChangeTier(u.Name, tier.Code); err != nil {
			return err
		}
	}
	// Update billing fields
	billing := &user.Billing{
		StripeCustomerID:            customerID,
		StripeSubscriptionID:        subscriptionID,
		StripeSubscriptionStatus:    stripe.SubscriptionStatus(status),
		StripeSubscriptionInterval:  stripe.PriceRecurringInterval(interval),
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
func (s *Server) fetchStripePrices() (map[string]int64, error) {
	log.Debug("Caching prices from Stripe API")
	priceMap := make(map[string]int64)
	prices, err := s.stripe.ListPrices(&stripe.PriceListParams{Active: stripe.Bool(true)})
	if err != nil {
		log.Warn("Fetching Stripe prices failed: %s", err.Error())
		return nil, err
	}
	for _, p := range prices {
		priceMap[p.ID] = p.UnitAmount
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
