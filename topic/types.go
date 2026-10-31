package topic

import (
	"time"
)

// Config configures the service
type Config struct {
	Store *Store // Where the rows are shared with the other nodes and kept across restarts; nil for memory only
}

// RateVisitor is the subscriber who pays for the pushes to a UnifiedPush topic: its user, if it
// was logged in, and its IP, masked to the server's prefix bits. Which of the two is charged is
// decided on every push (see package server), so a tier change applies without a new subscribe.
type RateVisitor struct {
	UserID string // Empty if the subscriber was anonymous
	IP     string // The masked IP, never empty
}

// Topic is a topic's cluster-wide state as this node knows it (see Service.Get)
type Topic struct {
	FirstSeenAt  time.Time    // Earliest observation on any node: a refused push, or the first subscribe
	SubscribedAt time.Time    // Last subscriber activity on any node; zero if there was none yet (the topic was only pushed to)
	RateVisitor  *RateVisitor // Who pays for the pushes; nil if nobody who may pay subscribed yet
}

// write is what an entry still has to write to the store; a rate visitor set here outranks a touch
type write uint8

const (
	writeNone        write = iota
	writeTouch             // The row as it is here (Touch, Keepalive, Note)
	writeRateVisitor       // A rate visitor set here (SetRateVisitor): until written, the read-back does not replace it
)

// entry is one topic's row as this node knows it: the store's row as last read, merged with what
// happened here since (see Service.applyNoLock for the rules)
type entry struct {
	firstSeenAt  int64  // Unix seconds
	subscribedAt int64  // Unix seconds; 0 if never
	userID       string // The rate visitor's user ID, empty if anonymous or none
	ip           string // The rate visitor's masked IP, empty if none
	updatedAt    int64  // When the row was last written, by any node (unix seconds); the prune goes by it
	write        write  // Unwritten change, if any
}
