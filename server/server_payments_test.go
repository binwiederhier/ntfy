package server

import (
	"encoding/json"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v74"
	"golang.org/x/time/rate"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	"io"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPayments_Tiers(t *testing.T) {
	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	c.VisitorRequestLimitReplenish = 12 * time.Hour
	c.CacheDuration = 13 * time.Hour
	c.AttachmentFileSizeLimit = 111
	c.VisitorAttachmentTotalSizeLimit = 222
	c.AttachmentExpiryDuration = 123 * time.Second
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("ListPrices", mock.Anything).
		Return([]*stripe.Price{
			{ID: "price_123", UnitAmount: 500},
			{ID: "price_124", UnitAmount: 5000},
			{ID: "price_456", UnitAmount: 1000},
			{ID: "price_457", UnitAmount: 10000},
			{ID: "price_999", UnitAmount: 9999},
		}, nil)

	// Create tiers
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:   "ti_1",
		Code: "admin",
		Name: "Admin",
	}))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                       "ti_123",
		Code:                     "pro",
		Name:                     "Pro",
		MessageLimit:             1000,
		MessageExpiryDuration:    time.Hour,
		EmailLimit:               123,
		ReservationLimit:         777,
		AttachmentFileSizeLimit:  999,
		AttachmentTotalSizeLimit: 888,
		AttachmentExpiryDuration: time.Minute,
		StripeMonthlyPriceID:     "price_123",
		StripeYearlyPriceID:      "price_124",
	}))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                       "ti_444",
		Code:                     "business",
		Name:                     "Business",
		MessageLimit:             2000,
		MessageExpiryDuration:    10 * time.Hour,
		EmailLimit:               123123,
		ReservationLimit:         777333,
		AttachmentFileSizeLimit:  999111,
		AttachmentTotalSizeLimit: 888111,
		AttachmentExpiryDuration: time.Hour,
		StripeMonthlyPriceID:     "price_456",
		StripeYearlyPriceID:      "price_457",
	}))
	response := request(t, s, "GET", "/v1/tiers", "", nil)
	require.Equal(t, 200, response.Code)
	var tiers []apiAccountBillingTier
	require.Nil(t, json.NewDecoder(response.Body).Decode(&tiers))
	require.Equal(t, 3, len(tiers))

	// Free tier
	tier := tiers[0]
	require.Equal(t, "", tier.Code)
	require.Equal(t, "", tier.Name)
	require.Equal(t, "ip", tier.Limits.Basis)
	require.Equal(t, int64(0), tier.Limits.Reservations)
	require.Equal(t, int64(2), tier.Limits.Messages) // :-(
	require.Equal(t, int64(13*3600), tier.Limits.MessagesExpiryDuration)
	require.Equal(t, int64(24), tier.Limits.Emails)
	require.Equal(t, int64(111), tier.Limits.AttachmentFileSize)
	require.Equal(t, int64(222), tier.Limits.AttachmentTotalSize)
	require.Equal(t, int64(123), tier.Limits.AttachmentExpiryDuration)

	// Admin tier is not included, because it is not paid!

	tier = tiers[1]
	require.Equal(t, "pro", tier.Code)
	require.Equal(t, "Pro", tier.Name)
	require.Equal(t, "tier", tier.Limits.Basis)
	require.Equal(t, int64(500), tier.Prices.Month)
	require.Equal(t, int64(5000), tier.Prices.Year)
	require.Equal(t, int64(777), tier.Limits.Reservations)
	require.Equal(t, int64(1000), tier.Limits.Messages)
	require.Equal(t, int64(3600), tier.Limits.MessagesExpiryDuration)
	require.Equal(t, int64(123), tier.Limits.Emails)
	require.Equal(t, int64(999), tier.Limits.AttachmentFileSize)
	require.Equal(t, int64(888), tier.Limits.AttachmentTotalSize)
	require.Equal(t, int64(60), tier.Limits.AttachmentExpiryDuration)

	tier = tiers[2]
	require.Equal(t, "business", tier.Code)
	require.Equal(t, "Business", tier.Name)
	require.Equal(t, int64(1000), tier.Prices.Month)
	require.Equal(t, int64(10000), tier.Prices.Year)
	require.Equal(t, "tier", tier.Limits.Basis)
	require.Equal(t, int64(777333), tier.Limits.Reservations)
	require.Equal(t, int64(2000), tier.Limits.Messages)
	require.Equal(t, int64(36000), tier.Limits.MessagesExpiryDuration)
	require.Equal(t, int64(123123), tier.Limits.Emails)
	require.Equal(t, int64(999111), tier.Limits.AttachmentFileSize)
	require.Equal(t, int64(888111), tier.Limits.AttachmentTotalSize)
	require.Equal(t, int64(3600), tier.Limits.AttachmentExpiryDuration)
}

func TestPayments_SubscriptionCreate_NotAStripeCustomer_Success(t *testing.T) {
	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("NewCheckoutSession", mock.Anything).
		Return(&stripe.CheckoutSession{URL: "https://billing.stripe.com/abc/def"}, nil)

	// Create tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                   "ti_123",
		Code:                 "pro",
		StripeMonthlyPriceID: "price_123",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	// Create subscription
	response := request(t, s, "POST", "/v1/account/billing/subscription", `{"tier": "pro", "interval": "month"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	redirectResponse, err := util.UnmarshalJSON[apiAccountBillingSubscriptionCreateResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, "https://billing.stripe.com/abc/def", redirectResponse.RedirectURL)
}

func TestPayments_SubscriptionCreate_StripeCustomer_Success(t *testing.T) {
	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("GetCustomer", "acct_123").
		Return(&stripe.Customer{Subscriptions: &stripe.SubscriptionList{}}, nil)
	stripeMock.
		On("NewCheckoutSession", mock.Anything).
		Return(&stripe.CheckoutSession{URL: "https://billing.stripe.com/abc/def"}, nil)

	// Create tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                   "ti_123",
		Code:                 "pro",
		StripeMonthlyPriceID: "price_123",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	u, err := s.userManager.User("phil")
	require.Nil(t, err)

	billing := &user.Billing{
		StripeCustomerID: "acct_123",
	}
	require.Nil(t, s.userManager.ChangeBilling(u.Name, billing))

	// Create subscription
	response := request(t, s, "POST", "/v1/account/billing/subscription", `{"tier": "pro", "interval": "month"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	redirectResponse, err := util.UnmarshalJSON[apiAccountBillingSubscriptionCreateResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, "https://billing.stripe.com/abc/def", redirectResponse.RedirectURL)
}

func TestPayments_AccountDelete_Cancels_Subscription(t *testing.T) {
	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.EnableSignup = true
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("CancelSubscription", "sub_123").
		Return(&stripe.Subscription{}, nil)

	// Create tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                   "ti_123",
		Code:                 "pro",
		StripeMonthlyPriceID: "price_123",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	u, err := s.userManager.User("phil")
	require.Nil(t, err)

	billing := &user.Billing{
		StripeCustomerID:     "acct_123",
		StripeSubscriptionID: "sub_123",
	}
	require.Nil(t, s.userManager.ChangeBilling(u.Name, billing))

	// Delete account
	rr := request(t, s, "DELETE", "/v1/account", `{"password": "phil"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 401, rr.Code)
}

func TestPayments_Checkout_Success_And_Increase_Rate_Limits_Reset_Visitor(t *testing.T) {
	// This test is too overloaded, but it's also a great end-to-end a test.
	//
	// It tests:
	// - A successful checkout flow (not a paying customer -> paying customer)
	// - Tier-changes reset the rate limits for the user
	// - The request limits for tier-less user and a tier-user
	// - The message limits for a tier-user

	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	c.VisitorRequestLimitBurst = 5
	c.VisitorRequestLimitReplenish = time.Hour
	c.CacheBatchSize = 500
	c.CacheBatchTimeout = time.Second
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Create a user with a Stripe subscription and 3 reservations
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                    "ti_123",
		Code:                  "starter",
		StripeMonthlyPriceID:  "price_1234",
		ReservationLimit:      1,
		MessageLimit:          220, // 220 * 5% = 11 requests before rate limiting kicks in
		MessageExpiryDuration: time.Hour,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser)) // No tier
	u, err := s.userManager.User("phil")
	require.Nil(t, err)

	// Define how the mock should react
	stripeMock.
		On("GetSession", "SOMETOKEN").
		Return(&stripe.CheckoutSession{
			ClientReferenceID: u.ID, // ntfy user ID
			Customer: &stripe.Customer{
				ID: "acct_5555",
			},
			Subscription: &stripe.Subscription{
				ID: "sub_1234",
			},
		}, nil)
	stripeMock.
		On("GetSubscription", "sub_1234").
		Return(&stripe.Subscription{
			ID:               "sub_1234",
			Status:           stripe.SubscriptionStatusActive,
			CurrentPeriodEnd: 123456789,
			CancelAt:         0,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{
					{
						Price: &stripe.Price{
							ID: "price_1234",
							Recurring: &stripe.PriceRecurring{
								Interval: stripe.PriceRecurringIntervalMonth,
							},
						},
					},
				},
			},
		}, nil)
	stripeMock.
		On("UpdateCustomer", "acct_5555", &stripe.CustomerParams{
			Params: stripe.Params{
				Metadata: map[string]string{
					"user_id":   u.ID,
					"user_name": u.Name,
				},
			},
		}).
		Return(&stripe.Customer{}, nil)

	// Send messages until rate limit of free tier is hit
	for i := 0; i < 5; i++ {
		rr := request(t, s, "PUT", "/mytopic", "some message", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)
	}
	rr := request(t, s, "PUT", "/mytopic", "some message", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 429, rr.Code)

	// Verify some "before-stats"
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Nil(t, u.Tier)
	require.Equal(t, "", u.Billing.StripeCustomerID)
	require.Equal(t, "", u.Billing.StripeSubscriptionID)
	require.Equal(t, stripe.SubscriptionStatus(""), u.Billing.StripeSubscriptionStatus)
	require.Equal(t, stripe.PriceRecurringInterval(""), u.Billing.StripeSubscriptionInterval)
	require.Equal(t, int64(0), u.Billing.StripeSubscriptionPaidUntil.Unix())
	require.Equal(t, int64(0), u.Billing.StripeSubscriptionCancelAt.Unix())
	require.Equal(t, int64(0), u.Stats.Messages) // Messages and emails are not persisted for no-tier users!
	require.Equal(t, int64(0), u.Stats.Emails)

	// Simulate Stripe success return URL call (no user context)
	rr = request(t, s, "GET", "/v1/account/billing/subscription/success/SOMETOKEN", "", nil)
	require.Equal(t, 303, rr.Code)

	// Verify that database columns were updated
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, "starter", u.Tier.Code) // Not "pro"
	require.Equal(t, "acct_5555", u.Billing.StripeCustomerID)
	require.Equal(t, "sub_1234", u.Billing.StripeSubscriptionID)
	require.Equal(t, stripe.SubscriptionStatusActive, u.Billing.StripeSubscriptionStatus)
	require.Equal(t, stripe.PriceRecurringIntervalMonth, u.Billing.StripeSubscriptionInterval)
	require.Equal(t, int64(123456789), u.Billing.StripeSubscriptionPaidUntil.Unix())
	require.Equal(t, int64(0), u.Billing.StripeSubscriptionCancelAt.Unix())
	require.Equal(t, int64(0), u.Stats.Messages)
	require.Equal(t, int64(0), u.Stats.Emails)

	// Now for the fun part: Verify that new rate limits are immediately applied
	// This only tests the request limiter, which kicks in before the message limiter.
	for i := 0; i < 11; i++ {
		rr := request(t, s, "PUT", "/mytopic", "some message", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code, "failed on iteration %d", i)
	}
	rr = request(t, s, "PUT", "/mytopic", "some message", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 429, rr.Code)

	// Now let's test the message limiter by faking a ridiculously generous rate limiter
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), u)
	v.requestLimiter = rate.NewLimiter(rate.Every(time.Millisecond), 1000000)

	var wg sync.WaitGroup
	for i := 0; i < 209; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rr := request(t, s, "PUT", "/mytopic", "some message", map[string]string{
				"Authorization": util.BasicAuth("phil", "phil"),
			})
			require.Equal(t, 200, rr.Code, "Failed on %d", i)
		}(i)
	}
	wg.Wait()
	rr = request(t, s, "PUT", "/mytopic", "some message", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 429, rr.Code)

	// And now let's cross-check that the stats are correct too
	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, int64(220), account.Limits.Messages)
	require.Equal(t, int64(220), account.Stats.Messages)
	require.Equal(t, int64(0), account.Stats.MessagesRemaining)
}

func TestPayments_Webhook_Subscription_Updated_Downgrade_From_PastDue_To_Active(t *testing.T) {
	t.Parallel()

	// This tests incoming webhooks from Stripe to update a subscription:
	// - All Stripe columns are updated in the user table
	// - When downgrading, excess reservations are deleted, including messages and attachments in
	//   the corresponding topics

	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("ConstructWebhookEvent", mock.Anything, "stripe signature", "webhook key").
		Return(jsonToStripeEvent(t, subscriptionUpdatedEventJSON), nil)

	// Create a user with a Stripe subscription and 3 reservations
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                       "ti_1",
		Code:                     "starter",
		StripeMonthlyPriceID:     "price_1234", // !
		ReservationLimit:         1,            // !
		MessageLimit:             100,
		MessageExpiryDuration:    time.Hour,
		AttachmentExpiryDuration: time.Hour,
		AttachmentFileSizeLimit:  1000000,
		AttachmentTotalSizeLimit: 1000000,
		AttachmentBandwidthLimit: 1000000,
	}))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                       "ti_2",
		Code:                     "pro",
		StripeMonthlyPriceID:     "price_1111", // !
		ReservationLimit:         3,            // !
		MessageLimit:             200,
		MessageExpiryDuration:    time.Hour,
		AttachmentExpiryDuration: time.Hour,
		AttachmentFileSizeLimit:  1000000,
		AttachmentTotalSizeLimit: 1000000,
		AttachmentBandwidthLimit: 1000000,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))
	require.Nil(t, s.userManager.AddReservation("phil", "atopic", user.PermissionDenyAll))
	require.Nil(t, s.userManager.AddReservation("phil", "ztopic", user.PermissionDenyAll))

	// Add billing details
	u, err := s.userManager.User("phil")
	require.Nil(t, err)

	billing := &user.Billing{
		StripeCustomerID:            "acct_5555",
		StripeSubscriptionID:        "sub_1234",
		StripeSubscriptionStatus:    stripe.SubscriptionStatusPastDue,
		StripeSubscriptionInterval:  stripe.PriceRecurringIntervalMonth,
		StripeSubscriptionPaidUntil: time.Unix(123, 0),
		StripeSubscriptionCancelAt:  time.Unix(456, 0),
	}
	require.Nil(t, s.userManager.ChangeBilling(u.Name, billing))

	// Add some messages to "atopic" and "ztopic", everything in "ztopic" will be deleted
	rr := request(t, s, "PUT", "/atopic", "some aaa message", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "PUT", "/atopic", strings.Repeat("a", 5000), map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	a2 := toMessage(t, rr.Body.String())
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, a2.ID))

	rr = request(t, s, "PUT", "/ztopic", "some zzz message", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "PUT", "/ztopic", strings.Repeat("z", 5000), map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	z2 := toMessage(t, rr.Body.String())
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, z2.ID))

	// Call the webhook: This does all the magic
	rr = request(t, s, "POST", "/v1/account/billing/webhook", "dummy", map[string]string{
		"Stripe-Signature": "stripe signature",
	})
	require.Equal(t, 200, rr.Code)

	// Verify that database columns were updated
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, "starter", u.Tier.Code) // Not "pro"
	require.Equal(t, "acct_5555", u.Billing.StripeCustomerID)
	require.Equal(t, "sub_1234", u.Billing.StripeSubscriptionID)
	require.Equal(t, stripe.SubscriptionStatusActive, u.Billing.StripeSubscriptionStatus)     // Not "past_due"
	require.Equal(t, stripe.PriceRecurringIntervalYear, u.Billing.StripeSubscriptionInterval) // Not "month"
	require.Equal(t, int64(1674268231), u.Billing.StripeSubscriptionPaidUntil.Unix())         // Updated
	require.Equal(t, int64(1674299999), u.Billing.StripeSubscriptionCancelAt.Unix())          // Updated

	// Verify that reservations were deleted
	r, err := s.userManager.Reservations("phil")
	require.Nil(t, err)
	require.Equal(t, 1, len(r)) // "ztopic" reservation was deleted
	require.Equal(t, "atopic", r[0].Topic)

	// Verify that messages and attachments were deleted
	time.Sleep(time.Second)
	s.execManager()

	ms, err := s.messageCache.Messages("atopic", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 2, len(ms))
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, a2.ID))

	ms, err = s.messageCache.Messages("ztopic", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 0, len(ms))
	require.NoFileExists(t, filepath.Join(s.config.AttachmentCacheDir, z2.ID))
}

func TestPayments_Webhook_Subscription_Deleted(t *testing.T) {
	// This tests incoming webhooks from Stripe to delete a subscription. It verifies that the database is
	// updated (all Stripe fields are deleted, and the tier is removed).
	//
	// It doesn't fully test the message/attachment deletion. That is tested above in the subscription update call.

	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("ConstructWebhookEvent", mock.Anything, "stripe signature", "webhook key").
		Return(jsonToStripeEvent(t, subscriptionDeletedEventJSON), nil)

	// Create a user with a Stripe subscription and 3 reservations
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                   "ti_1",
		Code:                 "pro",
		StripeMonthlyPriceID: "price_1234",
		ReservationLimit:     1,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))
	require.Nil(t, s.userManager.AddReservation("phil", "atopic", user.PermissionDenyAll))

	// Add billing details
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	require.Nil(t, s.userManager.ChangeBilling(u.Name, &user.Billing{
		StripeCustomerID:            "acct_5555",
		StripeSubscriptionID:        "sub_1234",
		StripeSubscriptionStatus:    stripe.SubscriptionStatusPastDue,
		StripeSubscriptionInterval:  stripe.PriceRecurringIntervalMonth,
		StripeSubscriptionPaidUntil: time.Unix(123, 0),
		StripeSubscriptionCancelAt:  time.Unix(0, 0),
	}))

	// Call the webhook: This does all the magic
	rr := request(t, s, "POST", "/v1/account/billing/webhook", "dummy", map[string]string{
		"Stripe-Signature": "stripe signature",
	})
	require.Equal(t, 200, rr.Code)

	// Verify that database columns were updated
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Nil(t, u.Tier)
	require.Equal(t, "acct_5555", u.Billing.StripeCustomerID)
	require.Equal(t, "", u.Billing.StripeSubscriptionID)
	require.Equal(t, stripe.SubscriptionStatus(""), u.Billing.StripeSubscriptionStatus)
	require.Equal(t, int64(0), u.Billing.StripeSubscriptionPaidUntil.Unix())
	require.Equal(t, int64(0), u.Billing.StripeSubscriptionCancelAt.Unix())

	// Verify that reservations were deleted
	r, err := s.userManager.Reservations("phil")
	require.Nil(t, err)
	require.Equal(t, 0, len(r))
}

func TestPayments_Subscription_Update_Different_Tier(t *testing.T) {
	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("GetSubscription", "sub_123").
		Return(&stripe.Subscription{
			ID: "sub_123",
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{
					{
						ID:    "someid_123",
						Price: &stripe.Price{ID: "price_123"},
					},
				},
			},
		}, nil)
	stripeMock.
		On("UpdateSubscription", "sub_123", &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(false),
			ProrationBehavior: stripe.String(string(stripe.SubscriptionSchedulePhaseProrationBehaviorAlwaysInvoice)),
			Items: []*stripe.SubscriptionItemsParams{
				{
					ID:    stripe.String("someid_123"),
					Price: stripe.String("price_457"),
				},
			},
		}).
		Return(&stripe.Subscription{}, nil)

	// Create tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                   "ti_123",
		Code:                 "pro",
		StripeMonthlyPriceID: "price_123",
		StripeYearlyPriceID:  "price_124",
	}))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		ID:                   "ti_456",
		Code:                 "business",
		StripeMonthlyPriceID: "price_456",
		StripeYearlyPriceID:  "price_457",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))
	require.Nil(t, s.userManager.ChangeBilling("phil", &user.Billing{
		StripeCustomerID:     "acct_123",
		StripeSubscriptionID: "sub_123",
	}))

	// Call endpoint to change subscription
	rr := request(t, s, "PUT", "/v1/account/billing/subscription", `{"tier":"business","interval":"year"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
}

func TestPayments_Subscription_Delete_At_Period_End(t *testing.T) {
	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("UpdateSubscription", "sub_123", mock.MatchedBy(func(s *stripe.SubscriptionParams) bool {
			return *s.CancelAtPeriodEnd // Is true
		})).
		Return(&stripe.Subscription{}, nil)

	// Create user
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeBilling("phil", &user.Billing{
		StripeCustomerID:     "acct_123",
		StripeSubscriptionID: "sub_123",
	}))

	// Delete subscription
	rr := request(t, s, "DELETE", "/v1/account/billing/subscription", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
}

func TestPayments_CreatePortalSession(t *testing.T) {
	stripeMock := &testStripeAPI{}
	defer stripeMock.AssertExpectations(t)

	c := newTestConfigWithAuthFile(t)
	c.StripeSecretKey = "secret key"
	c.StripeWebhookKey = "webhook key"
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Define how the mock should react
	stripeMock.
		On("NewPortalSession", &stripe.BillingPortalSessionParams{
			Customer:  stripe.String("acct_123"),
			ReturnURL: stripe.String(s.config.BaseURL),
		}).
		Return(&stripe.BillingPortalSession{
			URL: "https://billing.stripe.com/blablabla",
		}, nil)

	// Create user
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeBilling("phil", &user.Billing{
		StripeCustomerID:     "acct_123",
		StripeSubscriptionID: "sub_123",
	}))

	// Create portal session
	rr := request(t, s, "POST", "/v1/account/billing/portal", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	ps, _ := util.UnmarshalJSON[apiAccountBillingPortalRedirectResponse](io.NopCloser(rr.Body))
	require.Equal(t, "https://billing.stripe.com/blablabla", ps.RedirectURL)
}

type testStripeAPI struct {
	mock.Mock
}

var _ stripeAPI = (*testStripeAPI)(nil)

func (s *testStripeAPI) NewCheckoutSession(params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
	args := s.Called(params)
	return args.Get(0).(*stripe.CheckoutSession), args.Error(1)
}

func (s *testStripeAPI) NewPortalSession(params *stripe.BillingPortalSessionParams) (*stripe.BillingPortalSession, error) {
	args := s.Called(params)
	return args.Get(0).(*stripe.BillingPortalSession), args.Error(1)
}

func (s *testStripeAPI) ListPrices(params *stripe.PriceListParams) ([]*stripe.Price, error) {
	args := s.Called(params)
	return args.Get(0).([]*stripe.Price), args.Error(1)
}

func (s *testStripeAPI) GetCustomer(id string) (*stripe.Customer, error) {
	args := s.Called(id)
	return args.Get(0).(*stripe.Customer), args.Error(1)
}

func (s *testStripeAPI) GetSession(id string) (*stripe.CheckoutSession, error) {
	args := s.Called(id)
	return args.Get(0).(*stripe.CheckoutSession), args.Error(1)
}

func (s *testStripeAPI) GetSubscription(id string) (*stripe.Subscription, error) {
	args := s.Called(id)
	return args.Get(0).(*stripe.Subscription), args.Error(1)
}

func (s *testStripeAPI) UpdateCustomer(id string, params *stripe.CustomerParams) (*stripe.Customer, error) {
	args := s.Called(id, params)
	return args.Get(0).(*stripe.Customer), args.Error(1)
}

func (s *testStripeAPI) UpdateSubscription(id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
	args := s.Called(id, params)
	return args.Get(0).(*stripe.Subscription), args.Error(1)
}

func (s *testStripeAPI) CancelSubscription(id string) (*stripe.Subscription, error) {
	args := s.Called(id)
	return args.Get(0).(*stripe.Subscription), args.Error(1)
}

func (s *testStripeAPI) ConstructWebhookEvent(payload []byte, header string, secret string) (stripe.Event, error) {
	args := s.Called(payload, header, secret)
	return args.Get(0).(stripe.Event), args.Error(1)
}

func jsonToStripeEvent(t *testing.T, v string) stripe.Event {
	var e stripe.Event
	if err := json.Unmarshal([]byte(v), &e); err != nil {
		t.Fatal(err)
	}
	return e
}

const subscriptionUpdatedEventJSON = `
{
	"type": "customer.subscription.updated",
	"data": {
		"object": {
			"id": "sub_1234",
			"customer": "acct_5555",
			"status": "active",
			"current_period_end": 1674268231,
			"cancel_at": 1674299999,
			"items": {
				"data": [
					{
						"price": {
							"id": "price_1234",
							"recurring": {
								"interval": "year"
							}
						}
					}
				]
			}
		}
	}
}`

const subscriptionDeletedEventJSON = `
{
	"type": "customer.subscription.deleted",
	"data": {
		"object": {
			"id": "sub_1234",
			"customer": "acct_5555",
			"status": "active",
			"current_period_end": 1674268231,
			"cancel_at": 1674299999,
			"items": {
				"data": [
					{
						"price": {
							"id": "price_1234",
							"recurring": {
								"interval": "month"
							}
						}
					}
				]
			}
		}
	}
}`
