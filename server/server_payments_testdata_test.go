package server

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/util"
)

// knownStripeSubscriptionStatuses is the full set of statuses a Stripe subscription can report.
// stripeStatusGrantsPaidTier must give a defined answer for each, so an unexpected status from
// Stripe can never silently fall through.
var knownStripeSubscriptionStatuses = []string{
	"active", "trialing", "past_due", "unpaid", "incomplete", "incomplete_expired", "canceled", "paused",
}

// TestPayments_RealStripeEventTestdata runs anonymized real Stripe webhook events (captured from the
// live ntfy.sh account, with all IDs and any PII replaced) through the exact types and helpers the
// webhook handlers use. It guards the two assumptions the handlers rely on:
//   - a customer.subscription.deleted event carries the subscription's own "sub_" ID, so the
//     current-subscription match in handleAccountBillingWebhookSubscriptionDeleted can compare it
//     against the stored ID instead of purging on a customer match alone; and
//   - a customer.subscription.updated event carries a status that stripeStatusGrantsPaidTier
//     classifies, so the paid tier is only granted for a paying subscription.
func TestPayments_RealStripeEventTestdata(t *testing.T) {
	deleted := loadStripeEventObjects(t, "testdata/stripe/subscription_deleted")
	require.NotEmpty(t, deleted)
	for name, raw := range deleted {
		ev, err := util.UnmarshalJSON[apiStripeSubscriptionDeletedEvent](io.NopCloser(bytes.NewReader(raw)))
		require.NoError(t, err, name)
		require.True(t, strings.HasPrefix(ev.ID, "sub_"), "%s: deleted event ID %q is not a subscription ID; the stale-event match would skip real cancellations", name, ev.ID)
		require.True(t, strings.HasPrefix(ev.Customer, "cus_"), "%s: deleted event customer %q is not a customer ID", name, ev.Customer)
	}

	updated := loadStripeEventObjects(t, "testdata/stripe/subscription_updated")
	require.NotEmpty(t, updated)
	seenStatuses := make(map[string]bool)
	for name, raw := range updated {
		ev, err := util.UnmarshalJSON[apiStripeSubscriptionUpdatedEvent](io.NopCloser(bytes.NewReader(raw)))
		require.NoError(t, err, name)
		require.True(t, strings.HasPrefix(ev.ID, "sub_"), "%s: updated event ID %q is not a subscription ID", name, ev.ID)
		require.Contains(t, knownStripeSubscriptionStatuses, ev.Status, "%s: unexpected Stripe status %q; stripeStatusGrantsPaidTier would misclassify it", name, ev.Status)
		require.NotNil(t, ev.Items, "%s: updated event has no items", name)
		require.Len(t, ev.Items.Data, 1, "%s: updated event does not have exactly one line item", name)
		require.NotEmpty(t, ev.Items.Data[0].Price.ID, "%s: updated event has no price ID", name)
		seenStatuses[ev.Status] = true
	}
	// The curated testdata covers a withheld status and two granted ones, so the gate is exercised
	// against real statuses, not only the synthetic ones in the unit tests above.
	require.True(t, seenStatuses["incomplete_expired"], "testdata should include an incomplete_expired event (gate withholds the tier)")
	require.False(t, stripeStatusGrantsPaidTier("incomplete_expired"))
	require.True(t, seenStatuses["active"], "testdata should include an active event (gate grants the tier)")
	require.True(t, stripeStatusGrantsPaidTier("active"))
	require.True(t, seenStatuses["past_due"], "testdata should include a past_due event (gate keeps the tier)")
	require.True(t, stripeStatusGrantsPaidTier("past_due"))
}

// loadStripeEventObjects reads each *.json event file in dir and returns the raw bytes of its
// data.object, keyed by file name. That is exactly what ConstructWebhookEvent hands the handlers.
func loadStripeEventObjects(t *testing.T, dir string) map[string][]byte {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	objects := make(map[string][]byte, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		var event struct {
			Data struct {
				Object json.RawMessage `json:"object"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(raw, &event), f)
		require.NotEmpty(t, event.Data.Object, "%s: no data.object", f)
		objects[filepath.Base(f)] = event.Data.Object
	}
	return objects
}
