package topic

// State is one topic's row of the topic table, as this node knows it. Times are unix seconds.
type State struct {
	Topic             string
	FirstSeenAt       int64  // Earliest observation on any node: a refused push, or the first subscribe
	SubscribedAt      int64  // Last subscriber activity on any node; 0 if there was none yet
	RateVisitorUserID string // The user of the subscriber who pays for the pushes; empty if anonymous
	RateVisitorIP     string // Its IP, masked to the server's prefix bits; empty if nobody who may pay subscribed yet
	UpdatedAt         int64  // When the row was last written, by any node; the prune goes by it
}
