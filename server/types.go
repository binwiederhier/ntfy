package server

import (
	"net/http"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

// publishMessage is used as input when publishing as JSON
type publishMessage struct {
	Topic      string         `json:"topic"`
	SequenceID string         `json:"sequence_id"`
	Title      string         `json:"title"`
	Message    string         `json:"message"`
	Priority   int            `json:"priority"`
	Tags       []string       `json:"tags"`
	Click      string         `json:"click"`
	Icon       string         `json:"icon"`
	Actions    []model.Action `json:"actions"`
	Attach     string         `json:"attach"`
	Markdown   bool           `json:"markdown"`
	Filename   string         `json:"filename"`
	Email      string         `json:"email"`
	Call       string         `json:"call"`
	Cache      string         `json:"cache"`    // use string as it defaults to true (or use &bool instead)
	Firebase   string         `json:"firebase"` // use string as it defaults to true (or use &bool instead)
	Delay      string         `json:"delay"`
}

// messageEncoder is a function that knows how to encode a message
type messageEncoder func(msg *model.Message) (string, error)

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

func (q *queryFilter) Pass(msg *model.Message) bool {
	if msg.Event != model.MessageEvent && msg.Event != model.MessageDeleteEvent && msg.Event != model.MessageClearEvent {
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

// templateMode represents the mode in which templates are used
//
// It can be
// - empty: templating is disabled
// - a boolean string (yes/1/true/no/0/false): inline-templating mode
// - a filename (e.g. grafana): template mode with a file
type templateMode string

// Enabled returns true if templating is enabled
func (t templateMode) Enabled() bool {
	return t != ""
}

// InlineMode returns true if inline-templating mode is enabled
func (t templateMode) InlineMode() bool {
	return t.Enabled() && isBoolValue(string(t))
}

// FileMode returns true if file-templating mode is enabled
func (t templateMode) FileMode() bool {
	return t.Enabled() && !isBoolValue(string(t))
}

// FileName returns the filename if file-templating mode is enabled, or an empty string otherwise
func (t templateMode) FileName() string {
	if t.FileMode() {
		return string(t)
	}
	return ""
}

// templateFile represents a template file with title, message, and priority
// It is used for file-based templates, e.g. grafana, influxdb, etc.
//
// Example YAML:
//
//	  title: "Alert: {{ .Title }}"
//	  message: |
//		   This is a {{ .Type }} alert.
//		   It can be multiline.
//	  priority: '{{ if eq .status "Error" }}5{{ else }}3{{ end }}'
type templateFile struct {
	Title    *string `yaml:"title"`
	Message  *string `yaml:"message"`
	Priority *string `yaml:"priority"`
}

type apiHealthResponse struct {
	Healthy bool `json:"healthy"`
}

type apiVersionResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

type apiStatsResponse struct {
	Messages     int64   `json:"messages"`
	MessagesRate float64 `json:"messages_rate"` // Average number of messages per second
}

type apiUserAddOrUpdateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Hash     string `json:"hash"`
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
	Token       string `json:"token"`
	Label       string `json:"label,omitempty"`
	LastAccess  int64  `json:"last_access,omitempty"`
	LastOrigin  string `json:"last_origin,omitempty"`
	Expires     int64  `json:"expires,omitempty"`     // Unix timestamp
	Provisioned bool   `json:"provisioned,omitempty"` // True if this token was provisioned by the server config
}

type apiAccountPhoneNumberVerifyRequest struct {
	Number  string `json:"number"`
	Channel string `json:"channel"`
}

type apiAccountPhoneNumberAddRequest struct {
	Number string `json:"number"`
	Code   string `json:"code"` // Only set when adding a phone number
}

type apiAccountEmailVerifyRequest struct {
	Email string `json:"email"`
}

type apiAccountEmailAddRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
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
	Provisioned   bool                       `json:"provisioned,omitempty"`
	Language      string                     `json:"language,omitempty"`
	Notification  *user.NotificationPrefs    `json:"notification,omitempty"`
	Subscriptions []*user.Subscription       `json:"subscriptions,omitempty"`
	Reservations  []*apiAccountReservation   `json:"reservations,omitempty"`
	Tokens        []*apiAccountTokenResponse `json:"tokens,omitempty"`
	PhoneNumbers  []string                   `json:"phone_numbers,omitempty"`
	Emails        []string                   `json:"emails,omitempty"`
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
	RequireLogin       bool     `json:"require_login"`
	EnableSignup       bool     `json:"enable_signup"`
	EnablePayments     bool     `json:"enable_payments"`
	EnableCalls        bool     `json:"enable_calls"`
	EnableEmails       bool     `json:"enable_emails"`
	EnableEmailVerify  bool     `json:"enable_email_verify"`
	EnableReservations bool     `json:"enable_reservations"`
	EnableWebPush      bool     `json:"enable_web_push"`
	BillingContact     string   `json:"billing_contact"`
	WebPushPublicKey   string   `json:"web_push_public_key"`
	DisallowedTopics   []string `json:"disallowed_topics"`
	ConfigHash         string   `json:"config_hash"`
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
	Event          string         `json:"event"`
	SubscriptionID string         `json:"subscription_id"`
	Message        *model.Message `json:"message"`
}

func newWebPushPayload(subscriptionID string, message *model.Message) *webPushPayload {
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
