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

var (
	errNotAPaidTier                 = errors.New("tier does not have billing price identifier")
	errMultipleBillingSubscriptions = errors.New("cannot have multiple billing subscriptions")
	errNoBillingSubscription        = errors.New("user does not have an active billing subscription")
)

// handleBillingTiersGet returns all available paid tiers, and the free tier. This is to populate the upgrade dialog
// in the UI. Note that this endpoint does NOT have a user context (no v.user!).
func (s *Server) handleBillingTiersGet(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	tiers, err := s.userManager.Tiers()
	if err != nil {
		return err
	}
	freeTier := defaultVisitorLimits(s.config)
	response := []*apiAccountBillingTier{
		{
			// Free tier: no code, name or price
			Limits: &apiAccountLimits{
				Messages:                 freeTier.MessagesLimit,
				MessagesExpiryDuration:   int64(freeTier.MessagesExpiryDuration.Seconds()),
				Emails:                   freeTier.EmailsLimit,
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
				Messages:                 tier.MessagesLimit,
				MessagesExpiryDuration:   int64(tier.MessagesExpiryDuration.Seconds()),
				Emails:                   tier.EmailsLimit,
				Reservations:             tier.ReservationsLimit,
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
	if v.user.Billing.StripeSubscriptionID != "" {
		return errHTTPBadRequestBillingSubscriptionExists
	}
	req, err := readJSONWithLimit[apiAccountBillingSubscriptionChangeRequest](r.Body, jsonBodyBytesLimit)
	if err != nil {
		return err
	}
	tier, err := s.userManager.Tier(req.Tier)
	if err != nil {
		return err
	} else if tier.StripePriceID == "" {
		return errNotAPaidTier
	}
	log.Info("Stripe: No existing subscription, creating checkout flow")
	var stripeCustomerID *string
	if v.user.Billing.StripeCustomerID != "" {
		stripeCustomerID = &v.user.Billing.StripeCustomerID
		stripeCustomer, err := customer.Get(v.user.Billing.StripeCustomerID, nil)
		if err != nil {
			return err
		} else if stripeCustomer.Subscriptions != nil && len(stripeCustomer.Subscriptions.Data) > 0 {
			return errMultipleBillingSubscriptions
		}
	}
	successURL := s.config.BaseURL + apiAccountBillingSubscriptionCheckoutSuccessTemplate
	params := &stripe.CheckoutSessionParams{
		Customer:            stripeCustomerID, // A user may have previously deleted their subscription
		ClientReferenceID:   &v.user.Name,
		SuccessURL:          &successURL,
		Mode:                stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		AllowPromotionCodes: stripe.Bool(true),
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
	response := &apiAccountBillingSubscriptionCreateResponse{
		RedirectURL: sess.URL,
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleAccountBillingSubscriptionCreateSuccess(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	// We don't have a v.user in this endpoint, only a userManager!
	matches := apiAccountBillingSubscriptionCheckoutSuccessRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	sessionID := matches[1]
	sess, err := session.Get(sessionID, nil) // FIXME how do I rate limit this?
	if err != nil {
		log.Warn("Stripe: %s", err)
		return errHTTPBadRequestBillingRequestInvalid
	} else if sess.Customer == nil || sess.Subscription == nil || sess.ClientReferenceID == "" {
		return wrapErrHTTP(errHTTPBadRequestBillingRequestInvalid, "customer or subscription not found")
	}
	sub, err := subscription.Get(sess.Subscription.ID, nil)
	if err != nil {
		return err
	} else if sub.Items == nil || len(sub.Items.Data) != 1 || sub.Items.Data[0].Price == nil {
		return wrapErrHTTP(errHTTPBadRequestBillingRequestInvalid, "more than one line item in existing subscription")
	}
	tier, err := s.userManager.TierByStripePrice(sub.Items.Data[0].Price.ID)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(sess.ClientReferenceID)
	if err != nil {
		return err
	}
	if err := s.updateSubscriptionAndTier(u, sess.Customer.ID, sub.ID, string(sub.Status), sub.CurrentPeriodEnd, sub.CancelAt, tier.Code); err != nil {
		return err
	}
	http.Redirect(w, r, s.config.BaseURL+accountPath, http.StatusSeeOther)
	return nil
}

// handleAccountBillingSubscriptionUpdate updates an existing Stripe subscription to a new price, and updates
// a user's tier accordingly. This endpoint only works if there is an existing subscription.
func (s *Server) handleAccountBillingSubscriptionUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user.Billing.StripeSubscriptionID == "" {
		return errNoBillingSubscription
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
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccountBillingSubscriptionDelete facilitates downgrading a paid user to a tier-less user,
// and cancelling the Stripe subscription entirely
func (s *Server) handleAccountBillingSubscriptionDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if v.user.Billing.StripeSubscriptionID != "" {
		params := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(true),
		}
		_, err := subscription.Update(v.user.Billing.StripeSubscriptionID, params)
		if err != nil {
			return err
		}
	}
	return s.writeJSON(w, newSuccessResponse())
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
	return s.writeJSON(w, response)
}

// handleAccountBillingWebhook handles incoming Stripe webhooks. It mainly keeps the local user database in sync
// with the Stripe view of the world. This endpoint is authorized via the Stripe webhook secret. Note that the
// visitor (v) in this endpoint is the Stripe API, so we don't have v.user available.
func (s *Server) handleAccountBillingWebhook(w http.ResponseWriter, r *http.Request, _ *visitor) error {
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
	event, err := webhook.ConstructEvent(body.PeekedBytes, stripeSignature, s.config.StripeWebhookKey)
	if err != nil {
		return errHTTPBadRequestBillingRequestInvalid
	} else if event.Data == nil || event.Data.Raw == nil {
		return errHTTPBadRequestBillingRequestInvalid
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
	r, err := util.UnmarshalJSON[apiStripeSubscriptionUpdatedEvent](io.NopCloser(bytes.NewReader(event)))
	if err != nil {
		return err
	} else if r.ID == "" || r.Customer == "" || r.Status == "" || r.CurrentPeriodEnd == 0 || r.Items == nil || len(r.Items.Data) != 1 || r.Items.Data[0].Price == nil || r.Items.Data[0].Price.ID == "" {
		return errHTTPBadRequestBillingRequestInvalid
	}
	subscriptionID, priceID := r.ID, r.Items.Data[0].Price.ID
	log.Info("Stripe: customer %s: Updating subscription to status %s, with price %s", r.Customer, r.Status, priceID)
	u, err := s.userManager.UserByStripeCustomer(r.Customer)
	if err != nil {
		return err
	}
	tier, err := s.userManager.TierByStripePrice(priceID)
	if err != nil {
		return err
	}
	if err := s.updateSubscriptionAndTier(u, r.Customer, subscriptionID, r.Status, r.CurrentPeriodEnd, r.CancelAt, tier.Code); err != nil {
		return err
	}
	s.publishSyncEventAsync(s.visitorFromUser(u, netip.IPv4Unspecified()))
	return nil
}

func (s *Server) handleAccountBillingWebhookSubscriptionDeleted(event json.RawMessage) error {
	r, err := util.UnmarshalJSON[apiStripeSubscriptionDeletedEvent](io.NopCloser(bytes.NewReader(event)))
	if err != nil {
		return err
	} else if r.Customer == "" {
		return errHTTPBadRequestBillingRequestInvalid
	}
	log.Info("Stripe: customer %s: subscription deleted, downgrading to unpaid tier", r.Customer)
	u, err := s.userManager.UserByStripeCustomer(r.Customer)
	if err != nil {
		return err
	}
	if err := s.updateSubscriptionAndTier(u, r.Customer, "", "", 0, 0, ""); err != nil {
		return err
	}
	s.publishSyncEventAsync(s.visitorFromUser(u, netip.IPv4Unspecified()))
	return nil
}

func (s *Server) updateSubscriptionAndTier(u *user.User, customerID, subscriptionID, status string, paidUntil, cancelAt int64, tier string) error {
	u.Billing.StripeCustomerID = customerID
	u.Billing.StripeSubscriptionID = subscriptionID
	u.Billing.StripeSubscriptionStatus = stripe.SubscriptionStatus(status)
	u.Billing.StripeSubscriptionPaidUntil = time.Unix(paidUntil, 0)
	u.Billing.StripeSubscriptionCancelAt = time.Unix(cancelAt, 0)
	if tier == "" {
		if err := s.userManager.ResetTier(u.Name); err != nil {
			return err
		}
	} else {
		if err := s.userManager.ChangeTier(u.Name, tier); err != nil {
			return err
		}
	}
	if err := s.userManager.ChangeBilling(u); err != nil {
		return err
	}
	return nil
}

// fetchStripePrices contacts the Stripe API to retrieve all prices. This is used by the server to cache the prices
// in memory, and ultimately for the web app to display the price table.
func fetchStripePrices() (map[string]string, error) {
	log.Debug("Caching prices from Stripe API")
	prices := make(map[string]string)
	iter := price.List(&stripe.PriceListParams{
		Active: stripe.Bool(true),
	})
	for iter.Next() {
		p := iter.Price()
		if p.UnitAmount%100 == 0 {
			prices[p.ID] = fmt.Sprintf("$%d", p.UnitAmount/100)
		} else {
			prices[p.ID] = fmt.Sprintf("$%.2f", float64(p.UnitAmount)/100)
		}
		log.Trace("- Caching price %s = %v", p.ID, prices[p.ID])
	}
	if iter.Err() != nil {
		log.Warn("Fetching Stripe prices failed: %s", iter.Err().Error())
		return nil, iter.Err()
	}
	return prices, nil
}
