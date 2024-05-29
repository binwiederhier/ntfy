package server

import (
	"net/http"
	"net/netip"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"

	"heckel.io/ntfy/v2/util"
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
	ID          string      `json:"id"`                // Random message ID
	Time        int64       `json:"time"`              // Unix time in seconds
	Expires     int64       `json:"expires,omitempty"` // Unix time in seconds (not required for open/keepalive)
	Event       string      `json:"event"`             // One of the above
	Topic       string      `json:"topic"`
	Title       string      `json:"title,omitempty"`
	Message     string      `json:"message,omitempty"`
	Priority    int         `json:"priority,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Click       string      `json:"click,omitempty"`
	Icon        string      `json:"icon,omitempty"`
	Actions     []*action   `json:"actions,omitempty"`
	Attachment  *attachment `json:"attachment,omitempty"`
	PollID      string      `json:"poll_id,omitempty"`
	ContentType string      `json:"content_type,omitempty"` // text/plain by default (if empty), or text/markdown
	Encoding    string      `json:"encoding,omitempty"`     // empty for raw UTF-8, or "base64" for encoded bytes
	Sender      netip.Addr  `json:"-"`                      // IP address of uploader, used for rate limiting
	User        string      `json:"-"`                      // UserID of the uploader, used to associated attachments
}

func (m *message) Context() log.Context {
	fields := map[string]any{
		"topic":             m.Topic,
		"message_id":        m.ID,
		"message_time":      m.Time,
		"message_event":     m.Event,
		"message_body_size": len(m.Message),
	}
	if m.Sender.IsValid() {
		fields["message_sender"] = m.Sender.String()
	}
	if m.User != "" {
		fields["message_user"] = m.User
	}
	return fields
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
	Markdown bool     `json:"markdown"`
	Filename string   `json:"filename"`
	Email    string   `json:"email"`
	Call     string   `json:"call"`
	Cache    string   `json:"cache"`    // use string as it defaults to true (or use &bool instead)
	Firebase string   `json:"firebase"` // use string as it defaults to true (or use &bool instead)
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

type apiStatsResponse struct {
	Messages     int64   `json:"messages"`
	MessagesRate float64 `json:"messages_rate"` // Average number of messages per second
}

type apiUserAddRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Tier     string `json:"tier"`
	// Do not add 'role' here. We don't want to add admins via the API.
}

type apiUserResponse struct {
	Username string                  `json:"username"`
	Role     string                  `json:"role"`
	Tier     string                  `json:"tier,omitempty"`
	Grants   []*apiUserGrantResponse `json:"grants,omitempty"`
}

type apiUserGrantResponse struct {
	Topic      string `json:"topic"` // This may be a pattern
	Permission string `json:"permission"`
}

type apiUserDeleteRequest struct {
	Username string `json:"username"`
}

type apiAccessAllowRequest struct {
	Username   string `json:"username"`
	Topic      string `json:"topic"` // This may be a pattern
	Permission string `json:"permission"`
}

type apiAccessResetRequest struct {
	Username string `json:"username"`
	Topic    string `json:"topic"`
}

type apiAccountCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type apiAccountPasswordChangeRequest struct {
	Password    string `json:"password"`
	NewPassword string `json:"new_password"`
}

type apiAccountDeleteRequest struct {
	Password string `json:"password"`
}

type apiAccountTokenIssueRequest struct {
	Label   *string `json:"label"`
	Expires *int64  `json:"expires"` // Unix timestamp
}

type apiAccountTokenUpdateRequest struct {
	Token   string  `json:"token"`
	Label   *string `json:"label"`
	Expires *int64  `json:"expires"` // Unix timestamp
}

type apiAccountTokenResponse struct {
	Token      string `json:"token"`
	Label      string `json:"label,omitempty"`
	LastAccess int64  `json:"last_access,omitempty"`
	LastOrigin string `json:"last_origin,omitempty"`
	Expires    int64  `json:"expires,omitempty"` // Unix timestamp
}

type apiAccountPhoneNumberVerifyRequest struct {
	Number  string `json:"number"`
	Channel string `json:"channel"`
}

type apiAccountPhoneNumberAddRequest struct {
	Number string `json:"number"`
	Code   string `json:"code"` // Only set when adding a phone number
}

type apiAccountTier struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type apiAccountLimits struct {
	Basis                    string `json:"basis,omitempty"` // "ip" or "tier"
	Messages                 int64  `json:"messages"`
	MessagesExpiryDuration   int64  `json:"messages_expiry_duration"`
	Emails                   int64  `json:"emails"`
	Calls                    int64  `json:"calls"`
	Reservations             int64  `json:"reservations"`
	AttachmentTotalSize      int64  `json:"attachment_total_size"`
	AttachmentFileSize       int64  `json:"attachment_file_size"`
	AttachmentExpiryDuration int64  `json:"attachment_expiry_duration"`
	AttachmentBandwidth      int64  `json:"attachment_bandwidth"`
}

type apiAccountStats struct {
	Messages                     int64 `json:"messages"`
	MessagesRemaining            int64 `json:"messages_remaining"`
	Emails                       int64 `json:"emails"`
	EmailsRemaining              int64 `json:"emails_remaining"`
	Calls                        int64 `json:"calls"`
	CallsRemaining               int64 `json:"calls_remaining"`
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
	Interval     string `json:"interval,omitempty"`
	PaidUntil    int64  `json:"paid_until,omitempty"`
	CancelAt     int64  `json:"cancel_at,omitempty"`
}

type apiAccountResponse struct {
	Username      string                     `json:"username"`
	Role          string                     `json:"role,omitempty"`
	SyncTopic     string                     `json:"sync_topic,omitempty"`
	Language      string                     `json:"language,omitempty"`
	Notification  *user.NotificationPrefs    `json:"notification,omitempty"`
	Subscriptions []*user.Subscription       `json:"subscriptions,omitempty"`
	Reservations  []*apiAccountReservation   `json:"reservations,omitempty"`
	Tokens        []*apiAccountTokenResponse `json:"tokens,omitempty"`
	PhoneNumbers  []string                   `json:"phone_numbers,omitempty"`
	Tier          *apiAccountTier            `json:"tier,omitempty"`
	Limits        *apiAccountLimits          `json:"limits,omitempty"`
	Stats         *apiAccountStats           `json:"stats,omitempty"`
	Billing       *apiAccountBilling         `json:"billing,omitempty"`
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
	EnableCalls        bool     `json:"enable_calls"`
	EnableEmails       bool     `json:"enable_emails"`
	EnableReservations bool     `json:"enable_reservations"`
	EnableWebPush      bool     `json:"enable_web_push"`
	BillingContact     string   `json:"billing_contact"`
	WebPushPublicKey   string   `json:"web_push_public_key"`
	DisallowedTopics   []string `json:"disallowed_topics"`
}

type apiAccountBillingPrices struct {
	Month int64 `json:"month"`
	Year  int64 `json:"year"`
}

type apiAccountBillingTier struct {
	Code   string                   `json:"code,omitempty"`
	Name   string                   `json:"name,omitempty"`
	Prices *apiAccountBillingPrices `json:"prices,omitempty"`
	Limits *apiAccountLimits        `json:"limits"`
}

type apiAccountBillingSubscriptionCreateResponse struct {
	RedirectURL string `json:"redirect_url"`
}

type apiAccountBillingSubscriptionChangeRequest struct {
	Tier     string `json:"tier"`
	Interval string `json:"interval"`
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
				ID        string `json:"id"`
				Recurring *struct {
					Interval string `json:"interval"`
				} `json:"recurring"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

type apiStripeSubscriptionDeletedEvent struct {
	ID       string `json:"id"`
	Customer string `json:"customer"`
}

type apiWebPushUpdateSubscriptionRequest struct {
	Endpoint string   `json:"endpoint"`
	Auth     string   `json:"auth"`
	P256dh   string   `json:"p256dh"`
	Topics   []string `json:"topics"`
}

// List of possible Web Push events (see sw.js)
const (
	webPushMessageEvent  = "message"
	webPushExpiringEvent = "subscription_expiring"
)

type webPushPayload struct {
	Event          string   `json:"event"`
	SubscriptionID string   `json:"subscription_id"`
	Message        *message `json:"message"`
}

func newWebPushPayload(subscriptionID string, message *message) *webPushPayload {
	return &webPushPayload{
		Event:          webPushMessageEvent,
		SubscriptionID: subscriptionID,
		Message:        message,
	}
}

type webPushControlMessagePayload struct {
	Event string `json:"event"`
}

func newWebPushSubscriptionExpiringPayload() *webPushControlMessagePayload {
	return &webPushControlMessagePayload{
		Event: webPushExpiringEvent,
	}
}

type webPushSubscription struct {
	ID       string
	Endpoint string
	Auth     string
	P256dh   string
	UserID   string
}

func (w *webPushSubscription) Context() log.Context {
	return map[string]any{
		"web_push_subscription_id":       w.ID,
		"web_push_subscription_user_id":  w.UserID,
		"web_push_subscription_endpoint": w.Endpoint,
	}
}

// https://developer.mozilla.org/en-US/docs/Web/Manifest
type webManifestResponse struct {
	Name            string             `json:"name"`
	Description     string             `json:"description"`
	ShortName       string             `json:"short_name"`
	Scope           string             `json:"scope"`
	StartURL        string             `json:"start_url"`
	Display         string             `json:"display"`
	BackgroundColor string             `json:"background_color"`
	ThemeColor      string             `json:"theme_color"`
	Icons           []*webManifestIcon `json:"icons"`
}

type webManifestIcon struct {
	SRC   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}
