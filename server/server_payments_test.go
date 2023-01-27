package server

import (
	"encoding/json"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v74"
	"golang.org/x/time/rate"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
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
			{ID: "price_456", UnitAmount: 1000},
			{ID: "price_999", UnitAmount: 9999},
		}, nil)

	// Create tiers
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		ID:   "ti_1",
		Code: "admin",
		Name: "Admin",
	}))
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
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
		StripePriceID:            "price_123",
	}))
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
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
		StripePriceID:            "price_456",
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
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		ID:            "ti_123",
		Code:          "pro",
		StripePriceID: "price_123",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	// Create subscription
	response := request(t, s, "POST", "/v1/account/billing/subscription", `{"tier": "pro"}`, map[string]string{
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
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		ID:            "ti_123",
		Code:          "pro",
		StripePriceID: "price_123",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	u, err := s.userManager.User("phil")
	require.Nil(t, err)

	billing := &user.Billing{
		StripeCustomerID: "acct_123",
	}
	require.Nil(t, s.userManager.ChangeBilling(u.Name, billing))

	// Create subscription
	response := request(t, s, "POST", "/v1/account/billing/subscription", `{"tier": "pro"}`, map[string]string{
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
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		ID:            "ti_123",
		Code:          "pro",
		StripePriceID: "price_123",
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
	c.CacheStartupQueries = `
pragma journal_mode = WAL;
pragma synchronous = normal;
pragma temp_store = memory;
`
	c.CacheBatchSize = 500
	c.CacheBatchTimeout = time.Second
	s := newTestServer(t, c)
	s.stripe = stripeMock

	// Create a user with a Stripe subscription and 3 reservations
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		ID:                    "ti_123",
		Code:                  "starter",
		StripePriceID:         "price_1234",
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
						Price: &stripe.Price{ID: "price_1234"},
					},
				},
			},
		}, nil)
	stripeMock.
		On("UpdateCustomer", mock.Anything).
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
	require.Equal(t, int64(123456789), u.Billing.StripeSubscriptionPaidUntil.Unix())
	require.Equal(t, int64(0), u.Billing.StripeSubscriptionCancelAt.Unix())

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
		go func() {
			rr := request(t, s, "PUT", "/mytopic", "some message", map[string]string{
				"Authorization": util.BasicAuth("phil", "phil"),
			})
			require.Equal(t, 200, rr.Code)
			wg.Done()
		}()
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
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		ID:                       "ti_1",
		Code:                     "starter",
		StripePriceID:            "price_1234", // !
		ReservationLimit:         1,            // !
		MessageLimit:             100,
		MessageExpiryDuration:    time.Hour,
		AttachmentExpiryDuration: time.Hour,
		AttachmentFileSizeLimit:  1000000,
		AttachmentTotalSizeLimit: 1000000,
		AttachmentBandwidthLimit: 1000000,
	}))
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		ID:                       "ti_2",
		Code:                     "pro",
		StripePriceID:            "price_1111", // !
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
	require.Equal(t, stripe.SubscriptionStatusActive, u.Billing.StripeSubscriptionStatus) // Not "past_due"
	require.Equal(t, int64(1674268231), u.Billing.StripeSubscriptionPaidUntil.Unix())     // Updated
	require.Equal(t, int64(1674299999), u.Billing.StripeSubscriptionCancelAt.Unix())      // Updated

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
	args := s.Called(id)
	return args.Get(0).(*stripe.Customer), args.Error(1)
}

func (s *testStripeAPI) UpdateSubscription(id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
	args := s.Called(id)
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
							"id": "price_1234"
						}
					}
				]
			}
		}
	}
}`
