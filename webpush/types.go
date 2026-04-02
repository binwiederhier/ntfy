package webpush

import "heckel.io/ntfy/v2/log"

// Subscription represents a web push subscription.
type Subscription struct {
	ID       string
	Endpoint string
	Auth     string
	P256dh   string
	UserID   string
}

// Context returns the logging context for the subscription.
func (w *Subscription) Context() log.Context {
	return map[string]any{
		"web_push_subscription_id":       w.ID,
		"web_push_subscription_user_id":  w.UserID,
		"web_push_subscription_endpoint": w.Endpoint,
	}
}
