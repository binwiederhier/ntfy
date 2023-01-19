package server

import (
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v74"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"io"
	"testing"
)

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
		Code:          "pro",
		StripePriceID: "price_123",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, "unit-test"))

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
		Code:          "pro",
		StripePriceID: "price_123",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, "unit-test"))

	u, err := s.userManager.User("phil")
	require.Nil(t, err)

	u.Billing.StripeCustomerID = "acct_123"
	require.Nil(t, s.userManager.ChangeBilling(u))

	// Create subscription
	response := request(t, s, "POST", "/v1/account/billing/subscription", `{"tier": "pro"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	redirectResponse, err := util.UnmarshalJSON[apiAccountBillingSubscriptionCreateResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, "https://billing.stripe.com/abc/def", redirectResponse.RedirectURL)
}

type testStripeAPI struct {
	mock.Mock
}

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

func (s *testStripeAPI) UpdateSubscription(id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
	args := s.Called(id)
	return args.Get(0).(*stripe.Subscription), args.Error(1)
}

func (s *testStripeAPI) ConstructWebhookEvent(payload []byte, header string, secret string) (stripe.Event, error) {
	args := s.Called(payload, header, secret)
	return args.Get(0).(stripe.Event), args.Error(1)
}

var _ stripeAPI = (*testStripeAPI)(nil)
