package server

import (
	"heckel.io/ntfy/user"
	"net/http"
	"net/netip"
	"time"

	"heckel.io/ntfy/util"
)

// List of possible events
const (
	openEvent        = "open"
	keepaliveEvent   = "keepalive"
	messageEvent     = "message"
	pollRequestEvent = "poll_request"
)

const (
	messageIDLength = 12
)

// message represents a message published to a topic
type message struct {
	ID         string      `json:"id"`                // Random message ID
	Time       int64       `json:"time"`              // Unix time in seconds
	Expires    int64       `json:"expires,omitempty"` // Unix time in seconds (not required for open/keepalive)
	Event      string      `json:"event"`             // One of the above
	Topic      string      `json:"topic"`
	Title      string      `json:"title,omitempty"`
	Message    string      `json:"message,omitempty"`
	Priority   int         `json:"priority,omitempty"`
	Tags       []string    `json:"tags,omitempty"`
	Click      string      `json:"click,omitempty"`
	Icon       string      `json:"icon,omitempty"`
	Actions    []*action   `json:"actions,omitempty"`
	Attachment *attachment `json:"attachment,omitempty"`
	PollID     string      `json:"poll_id,omitempty"`
	Encoding   string      `json:"encoding,omitempty"` // empty for raw UTF-8, or "base64" for encoded bytes
	Sender     netip.Addr  `json:"-"`                  // IP address of uploader, used for rate limiting
	User       string      `json:"-"`                  // Username of the uploader, used to associated attachments
}

type attachment struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	URL     string `json:"url"`
}

type action struct {
	ID      string            `json:"id"`
	Action  string            `json:"action"`            // "view", "broadcast", or "http"
	Label   string            `json:"label"`             // action button label
	Clear   bool              `json:"clear"`             // clear notification after successful execution
	URL     string            `json:"url,omitempty"`     // used in "view" and "http" actions
	Method  string            `json:"method,omitempty"`  // used in "http" action, default is POST (!)
	Headers map[string]string `json:"headers,omitempty"` // used in "http" action
	Body    string            `json:"body,omitempty"`    // used in "http" action
	Intent  string            `json:"intent,omitempty"`  // used in "broadcast" action
	Extras  map[string]string `json:"extras,omitempty"`  // used in "broadcast" action
}

func newAction() *action {
	return &action{
		Headers: make(map[string]string),
		Extras:  make(map[string]string),
	}
}

// publishMessage is used as input when publishing as JSON
type publishMessage struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority int      `json:"priority"`
	Tags     []string `json:"tags"`
	Click    string   `json:"click"`
	Icon     string   `json:"icon"`
	Actions  []action `json:"actions"`
	Attach   string   `json:"attach"`
	Filename string   `json:"filename"`
	Email    string   `json:"email"`
	Delay    string   `json:"delay"`
}

// messageEncoder is a function that knows how to encode a message
type messageEncoder func(msg *message) (string, error)

// newMessage creates a new message with the current timestamp
func newMessage(event, topic, msg string) *message {
	return &message{
		ID:      util.RandomString(messageIDLength),
		Time:    time.Now().Unix(),
		Event:   event,
		Topic:   topic,
		Message: msg,
	}
}

// newOpenMessage is a convenience method to create an open message
func newOpenMessage(topic string) *message {
	return newMessage(openEvent, topic, "")
}

// newKeepaliveMessage is a convenience method to create a keepalive message
func newKeepaliveMessage(topic string) *message {
	return newMessage(keepaliveEvent, topic, "")
}

// newDefaultMessage is a convenience method to create a notification message
func newDefaultMessage(topic, msg string) *message {
	return newMessage(messageEvent, topic, msg)
}

// newPollRequestMessage is a convenience method to create a poll request message
func newPollRequestMessage(topic, pollID string) *message {
	m := newMessage(pollRequestEvent, topic, newMessageBody)
	m.PollID = pollID
	return m
}

func validMessageID(s string) bool {
	return util.ValidRandomString(s, messageIDLength)
}

type sinceMarker struct {
	time time.Time
	id   string
}

func newSinceTime(timestamp int64) sinceMarker {
	return sinceMarker{time.Unix(timestamp, 0), ""}
}

func newSinceID(id string) sinceMarker {
	return sinceMarker{time.Unix(0, 0), id}
}

func (t sinceMarker) IsAll() bool {
	return t == sinceAllMessages
}

func (t sinceMarker) IsNone() bool {
	return t == sinceNoMessages
}

func (t sinceMarker) IsID() bool {
	return t.id != ""
}

func (t sinceMarker) Time() time.Time {
	return t.time
}

func (t sinceMarker) ID() string {
	return t.id
}

var (
	sinceAllMessages = sinceMarker{time.Unix(0, 0), ""}
	sinceNoMessages  = sinceMarker{time.Unix(1, 0), ""}
)

type queryFilter struct {
	ID       string
	Message  string
	Title    string
	Tags     []string
	Priority []int
}

func parseQueryFilters(r *http.Request) (*queryFilter, error) {
	idFilter := readParam(r, "x-id", "id")
	messageFilter := readParam(r, "x-message", "message", "m")
	titleFilter := readParam(r, "x-title", "title", "t")
	tagsFilter := util.SplitNoEmpty(readParam(r, "x-tags", "tags", "tag", "ta"), ",")
	priorityFilter := make([]int, 0)
	for _, p := range util.SplitNoEmpty(readParam(r, "x-priority", "priority", "prio", "p"), ",") {
		priority, err := util.ParsePriority(p)
		if err != nil {
			return nil, errHTTPBadRequestPriorityInvalid
		}
		priorityFilter = append(priorityFilter, priority)
	}
	return &queryFilter{
		ID:       idFilter,
		Message:  messageFilter,
		Title:    titleFilter,
		Tags:     tagsFilter,
		Priority: priorityFilter,
	}, nil
}

func (q *queryFilter) Pass(msg *message) bool {
	if msg.Event != messageEvent {
		return true // filters only apply to messages
	} else if q.ID != "" && msg.ID != q.ID {
		return false
	} else if q.Message != "" && msg.Message != q.Message {
		return false
	} else if q.Title != "" && msg.Title != q.Title {
		return false
	}
	messagePriority := msg.Priority
	if messagePriority == 0 {
		messagePriority = 3 // For query filters, default priority (3) is the same as "not set" (0)
	}
	if len(q.Priority) > 0 && !util.Contains(q.Priority, messagePriority) {
		return false
	}
	if len(q.Tags) > 0 && !util.ContainsAll(msg.Tags, q.Tags) {
		return false
	}
	return true
}

type apiHealthResponse struct {
	Healthy bool `json:"healthy"`
}

type apiAccountCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type apiAccountPasswordChangeRequest struct {
	Password    string `json:"password"`
	NewPassword string `json:"new_password"`
}

type apiAccountTokenResponse struct {
	Token   string `json:"token"`
	Expires int64  `json:"expires"`
}

type apiAccountTier struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type apiAccountLimits struct {
	Basis                    string `json:"basis,omitempty"` // "ip", "role" or "tier"
	Messages                 int64  `json:"messages"`
	MessagesExpiryDuration   int64  `json:"messages_expiry_duration"`
	Emails                   int64  `json:"emails"`
	Reservations             int64  `json:"reservations"`
	AttachmentTotalSize      int64  `json:"attachment_total_size"`
	AttachmentFileSize       int64  `json:"attachment_file_size"`
	AttachmentExpiryDuration int64  `json:"attachment_expiry_duration"`
}

type apiAccountStats struct {
	Messages                     int64 `json:"messages"`
	MessagesRemaining            int64 `json:"messages_remaining"`
	Emails                       int64 `json:"emails"`
	EmailsRemaining              int64 `json:"emails_remaining"`
	Reservations                 int64 `json:"reservations"`
	ReservationsRemaining        int64 `json:"reservations_remaining"`
	AttachmentTotalSize          int64 `json:"attachment_total_size"`
	AttachmentTotalSizeRemaining int64 `json:"attachment_total_size_remaining"`
}

type apiAccountReservation struct {
	Topic    string `json:"topic"`
	Everyone string `json:"everyone"`
}

type apiAccountBilling struct {
	Customer     bool   `json:"customer"`
	Subscription bool   `json:"subscription"`
	Status       string `json:"status,omitempty"`
	PaidUntil    int64  `json:"paid_until,omitempty"`
	CancelAt     int64  `json:"cancel_at,omitempty"`
}

type apiAccountResponse struct {
	Username      string                   `json:"username"`
	Role          string                   `json:"role,omitempty"`
	SyncTopic     string                   `json:"sync_topic,omitempty"`
	Language      string                   `json:"language,omitempty"`
	Notification  *user.NotificationPrefs  `json:"notification,omitempty"`
	Subscriptions []*user.Subscription     `json:"subscriptions,omitempty"`
	Reservations  []*apiAccountReservation `json:"reservations,omitempty"`
	Tier          *apiAccountTier          `json:"tier,omitempty"`
	Limits        *apiAccountLimits        `json:"limits,omitempty"`
	Stats         *apiAccountStats         `json:"stats,omitempty"`
	Billing       *apiAccountBilling       `json:"billing,omitempty"`
}

type apiAccountReservationRequest struct {
	Topic    string `json:"topic"`
	Everyone string `json:"everyone"`
}

type apiConfigResponse struct {
	BaseURL            string   `json:"base_url"`
	AppRoot            string   `json:"app_root"`
	EnableLogin        bool     `json:"enable_login"`
	EnableSignup       bool     `json:"enable_signup"`
	EnablePayments     bool     `json:"enable_payments"`
	EnableReservations bool     `json:"enable_reservations"`
	DisallowedTopics   []string `json:"disallowed_topics"`
}

type apiAccountBillingTier struct {
	Code   string            `json:"code,omitempty"`
	Name   string            `json:"name,omitempty"`
	Price  string            `json:"price,omitempty"`
	Limits *apiAccountLimits `json:"limits"`
}

type apiAccountBillingSubscriptionCreateResponse struct {
	RedirectURL string `json:"redirect_url"`
}

type apiAccountBillingSubscriptionChangeRequest struct {
	Tier string `json:"tier"`
}

type apiAccountBillingPortalRedirectResponse struct {
	RedirectURL string `json:"redirect_url"`
}

type apiAccountSyncTopicResponse struct {
	Event string `json:"event"`
}

type apiSuccessResponse struct {
	Success bool `json:"success"`
}

func newSuccessResponse() *apiSuccessResponse {
	return &apiSuccessResponse{
		Success: true,
	}
}

type apiStripeSubscriptionUpdatedEvent struct {
	ID               string `json:"id"`
	Customer         string `json:"customer"`
	Status           string `json:"status"`
	CurrentPeriodEnd int64  `json:"current_period_end"`
	CancelAt         int64  `json:"cancel_at"`
	Items            *struct {
		Data []*struct {
			Price *struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

type apiStripeSubscriptionDeletedEvent struct {
	ID       string `json:"id"`
	Customer string `json:"customer"`
}
