package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/log"
)

// errHTTP is a generic HTTP error for any non-200 HTTP error
type errHTTP struct {
	Code     int    `json:"code,omitempty"`
	HTTPCode int    `json:"http"`
	Message  string `json:"error"`
	Link     string `json:"link,omitempty"`
	context  log.Context
	decision *limit.Decision // The limit decision behind a 429, sent as Retry-After and RateLimit headers; nil if none
}

func (e errHTTP) Error() string {
	return e.Message
}

func (e errHTTP) JSON() string {
	b, _ := json.Marshal(&e)
	return string(b)
}

func (e errHTTP) Context() log.Context {
	context := log.Context{
		"error":       e.Message,
		"error_code":  e.Code,
		"http_status": e.HTTPCode,
	}
	for k, v := range e.context {
		context[k] = v
	}
	return context
}

func (e errHTTP) Wrap(message string, args ...any) *errHTTP {
	clone := e.clone()
	clone.Message = fmt.Sprintf("%s; %s", clone.Message, fmt.Sprintf(message, args...))
	return &clone
}

func (e errHTTP) With(contexters ...log.Contexter) *errHTTP {
	c := e.clone()
	if c.context == nil {
		c.context = make(log.Context)
	}
	for _, contexter := range contexters {
		c.context.Merge(contexter.Context())
	}
	return &c
}

func (e errHTTP) Fields(context log.Context) *errHTTP {
	c := e.clone()
	if c.context == nil {
		c.context = make(log.Context)
	}
	c.context.Merge(context)
	return &c
}

// WithLimitDecision returns a copy of the error that carries the limit decision that denied the
// request, so the response tells the client when to retry and how much of the limit is left
func (e errHTTP) WithLimitDecision(d limit.Decision) *errHTTP {
	c := e.clone()
	c.decision = &d
	return &c
}

func (e errHTTP) clone() errHTTP {
	context := make(log.Context)
	for k, v := range e.context {
		context[k] = v
	}
	return errHTTP{
		Code:     e.Code,
		HTTPCode: e.HTTPCode,
		Message:  e.Message,
		Link:     e.Link,
		context:  context,
		decision: e.decision,
	}
}

// retryAfterSeconds is the Retry-After header value: whole seconds rounded up, so a client never
// retries too early, or "" if the wait is unknown
func (e errHTTP) retryAfterSeconds() string {
	if e.decision == nil || e.decision.RetryAfter <= 0 {
		return ""
	}
	return strconv.FormatInt(int64(math.Ceil(e.decision.RetryAfter.Seconds())), 10)
}

// errWebSocketPostUpgrade is a wrapper error indicating an error occurred after the WebSocket
// upgrade completed (i.e., the connection was hijacked). This is used to avoid calling
// WriteHeader on hijacked connections, which causes log spam.
type errWebSocketPostUpgrade struct {
	err error
}

func (e *errWebSocketPostUpgrade) Error() string {
	return e.err.Error()
}

func (e *errWebSocketPostUpgrade) Unwrap() error {
	return e.err
}

var (
	errHTTPBadRequest                                = &errHTTP{40000, http.StatusBadRequest, "invalid request", "", nil, nil}
	errHTTPBadRequestEmailDisabled                   = &errHTTP{40001, http.StatusBadRequest, "e-mail notifications are not enabled", "https://ntfy.sh/docs/config/#e-mail-notifications", nil, nil}
	errHTTPBadRequestDelayNoCache                    = &errHTTP{40002, http.StatusBadRequest, "cannot disable cache for delayed message", "", nil, nil}
	errHTTPBadRequestDelayNoEmail                    = &errHTTP{40003, http.StatusBadRequest, "delayed e-mail notifications are not supported", "", nil, nil}
	errHTTPBadRequestDelayCannotParse                = &errHTTP{40004, http.StatusBadRequest, "invalid delay parameter: unable to parse delay", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil, nil}
	errHTTPBadRequestDelayTooSmall                   = &errHTTP{40005, http.StatusBadRequest, "invalid delay parameter: too small, please refer to the docs", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil, nil}
	errHTTPBadRequestDelayTooLarge                   = &errHTTP{40006, http.StatusBadRequest, "invalid delay parameter: too large, please refer to the docs", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil, nil}
	errHTTPBadRequestPriorityInvalid                 = &errHTTP{40007, http.StatusBadRequest, "invalid priority parameter", "https://ntfy.sh/docs/publish/#message-priority", nil, nil}
	errHTTPBadRequestSinceInvalid                    = &errHTTP{40008, http.StatusBadRequest, "invalid since parameter", "https://ntfy.sh/docs/subscribe/api/#fetch-cached-messages", nil, nil}
	errHTTPBadRequestTopicInvalid                    = &errHTTP{40009, http.StatusBadRequest, "invalid request: topic invalid", "", nil, nil}
	errHTTPBadRequestTopicDisallowed                 = &errHTTP{40010, http.StatusBadRequest, "invalid request: topic name is not allowed", "", nil, nil}
	errHTTPBadRequestMessageNotUTF8                  = &errHTTP{40011, http.StatusBadRequest, "invalid request: message must be UTF-8 encoded", "", nil, nil}
	errHTTPBadRequestAttachmentURLInvalid            = &errHTTP{40013, http.StatusBadRequest, "invalid request: attachment URL is invalid", "https://ntfy.sh/docs/publish/#attachments", nil, nil}
	errHTTPBadRequestAttachmentsDisallowed           = &errHTTP{40014, http.StatusBadRequest, "invalid request: attachments not allowed", "https://ntfy.sh/docs/config/#attachments", nil, nil}
	errHTTPBadRequestAttachmentsExpiryBeforeDelivery = &errHTTP{40015, http.StatusBadRequest, "invalid request: attachment expiry before delayed delivery date", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil, nil}
	errHTTPBadRequestWebSocketsUpgradeHeaderMissing  = &errHTTP{40016, http.StatusBadRequest, "invalid request: client not using the websocket protocol", "https://ntfy.sh/docs/subscribe/api/#websockets", nil, nil}
	errHTTPBadRequestMessageJSONInvalid              = &errHTTP{40017, http.StatusBadRequest, "invalid request: request body must be message JSON", "https://ntfy.sh/docs/publish/#publish-as-json", nil, nil}
	errHTTPBadRequestActionsInvalid                  = &errHTTP{40018, http.StatusBadRequest, "invalid request: actions invalid", "https://ntfy.sh/docs/publish/#action-buttons", nil, nil}
	errHTTPBadRequestMatrixMessageInvalid            = &errHTTP{40019, http.StatusBadRequest, "invalid request: Matrix JSON invalid", "https://ntfy.sh/docs/publish/#matrix-gateway", nil, nil}
	errHTTPBadRequestIconURLInvalid                  = &errHTTP{40021, http.StatusBadRequest, "invalid request: icon URL is invalid", "https://ntfy.sh/docs/publish/#icons", nil, nil}
	errHTTPBadRequestSignupNotEnabled                = &errHTTP{40022, http.StatusBadRequest, "invalid request: signup not enabled", "https://ntfy.sh/docs/config", nil, nil}
	errHTTPBadRequestNoTokenProvided                 = &errHTTP{40023, http.StatusBadRequest, "invalid request: no token provided", "", nil, nil}
	errHTTPBadRequestJSONInvalid                     = &errHTTP{40024, http.StatusBadRequest, "invalid request: request body must be valid JSON", "", nil, nil}
	errHTTPBadRequestPermissionInvalid               = &errHTTP{40025, http.StatusBadRequest, "invalid request: incorrect permission string", "", nil, nil}
	errHTTPBadRequestIncorrectPasswordConfirmation   = &errHTTP{40026, http.StatusBadRequest, "invalid request: password confirmation is not correct", "", nil, nil}
	errHTTPBadRequestNotAPaidUser                    = &errHTTP{40027, http.StatusBadRequest, "invalid request: not a paid user", "", nil, nil}
	errHTTPBadRequestBillingRequestInvalid           = &errHTTP{40028, http.StatusBadRequest, "invalid request: not a valid billing request", "", nil, nil}
	errHTTPBadRequestBillingSubscriptionExists       = &errHTTP{40029, http.StatusBadRequest, "invalid request: billing subscription already exists", "", nil, nil}
	errHTTPBadRequestTierInvalid                     = &errHTTP{40030, http.StatusBadRequest, "invalid request: tier does not exist", "", nil, nil}
	errHTTPBadRequestUserNotFound                    = &errHTTP{40031, http.StatusBadRequest, "invalid request: user does not exist", "", nil, nil}
	errHTTPBadRequestPhoneCallsDisabled              = &errHTTP{40032, http.StatusBadRequest, "invalid request: calling is disabled", "https://ntfy.sh/docs/config/#phone-calls", nil, nil}
	errHTTPBadRequestPhoneNumberInvalid              = &errHTTP{40033, http.StatusBadRequest, "invalid request: phone number invalid", "https://ntfy.sh/docs/publish/#phone-calls", nil, nil}
	errHTTPBadRequestPhoneNumberNotVerified          = &errHTTP{40034, http.StatusBadRequest, "invalid request: phone number not verified, or no matching verified numbers found", "https://ntfy.sh/docs/publish/#phone-calls", nil, nil}
	errHTTPBadRequestAnonymousCallsNotAllowed        = &errHTTP{40035, http.StatusBadRequest, "invalid request: anonymous phone calls are not allowed", "https://ntfy.sh/docs/publish/#phone-calls", nil, nil}
	errHTTPBadRequestPhoneNumberVerifyChannelInvalid = &errHTTP{40036, http.StatusBadRequest, "invalid request: verification channel must be 'sms' or 'call'", "https://ntfy.sh/docs/publish/#phone-calls", nil, nil}
	errHTTPBadRequestDelayNoCall                     = &errHTTP{40037, http.StatusBadRequest, "invalid request: delayed call notifications are not supported", "", nil, nil}
	errHTTPBadRequestWebPushSubscriptionInvalid      = &errHTTP{40038, http.StatusBadRequest, "invalid request: web push payload malformed", "", nil, nil}
	errHTTPBadRequestWebPushEndpointUnknown          = &errHTTP{40039, http.StatusBadRequest, "invalid request: web push endpoint unknown", "", nil, nil}
	errHTTPBadRequestWebPushTopicCountTooHigh        = &errHTTP{40040, http.StatusBadRequest, "invalid request: too many web push topic subscriptions", "", nil, nil}
	errHTTPBadRequestTemplateMessageTooLarge         = &errHTTP{40041, http.StatusBadRequest, "invalid request: message or title is too large after replacing template", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestTemplateMessageNotJSON          = &errHTTP{40042, http.StatusBadRequest, "invalid request: message body must be JSON if templating is enabled", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestTemplateInvalid                 = &errHTTP{40043, http.StatusBadRequest, "invalid request: could not parse template", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestTemplateDisallowedFunctionCalls = &errHTTP{40044, http.StatusBadRequest, "invalid request: template contains disallowed function calls, e.g. template, call, define, or block", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestTemplateExecuteFailed           = &errHTTP{40045, http.StatusBadRequest, "invalid request: template execution failed", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestTemplateExecutionTimeout        = &errHTTP{40055, http.StatusBadRequest, "invalid request: template execution timed out", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestInvalidUsername                 = &errHTTP{40046, http.StatusBadRequest, "invalid request: invalid username", "", nil, nil}
	errHTTPBadRequestTemplateFileNotFound            = &errHTTP{40047, http.StatusBadRequest, "invalid request: template file not found", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestTemplateFileInvalid             = &errHTTP{40048, http.StatusBadRequest, "invalid request: template file invalid", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestSequenceIDInvalid               = &errHTTP{40049, http.StatusBadRequest, "invalid request: sequence ID invalid", "https://ntfy.sh/docs/publish/#updating-deleting-notifications", nil, nil}
	errHTTPBadRequestEmailAddressInvalid             = &errHTTP{40050, http.StatusBadRequest, "invalid request: invalid e-mail address", "https://ntfy.sh/docs/publish/#e-mail-notifications", nil, nil}
	errHTTPBadRequestEmailVerificationLinkInvalid    = &errHTTP{40051, http.StatusBadRequest, "invalid request: email verification link invalid or expired", "", nil, nil}
	errHTTPBadRequestEmailAddressNotVerified         = &errHTTP{40052, http.StatusBadRequest, "invalid request: email address not verified", "https://ntfy.sh/docs/publish/#e-mail-notifications", nil, nil}
	errHTTPBadRequestAnonymousEmailNotAllowed        = &errHTTP{40053, http.StatusBadRequest, "invalid request: anonymous email sending is not allowed", "https://ntfy.sh/docs/publish/#e-mail-notifications", nil, nil}
	errHTTPBadRequestResetLinkInvalid                = &errHTTP{40054, http.StatusBadRequest, "invalid request: password reset link invalid or expired", "", nil, nil}
	errHTTPBadRequestTemplateTooLarge                = &errHTTP{40056, http.StatusBadRequest, "invalid request: template too large", "https://ntfy.sh/docs/publish/#message-templating", nil, nil}
	errHTTPBadRequestTitleTooLarge                   = &errHTTP{40057, http.StatusBadRequest, "invalid request: title is too large", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPBadRequestTagsTooLarge                    = &errHTTP{40058, http.StatusBadRequest, "invalid request: tags are too large", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPBadRequestTimezoneInvalid                 = &errHTTP{40059, http.StatusBadRequest, "invalid request: invalid timezone", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil, nil}
	errHTTPBadRequestPhoneVerificationCodeInvalid    = &errHTTP{40060, http.StatusBadRequest, "invalid request: phone number verification code invalid", "", nil, nil}
	errHTTPBadRequestTooManyTopics                   = &errHTTP{40061, http.StatusBadRequest, "invalid request: too many topics", "https://ntfy.sh/docs/subscribe/api/#subscribe-to-multiple-topics", nil, nil}
	errHTTPBadRequestTierCodeInvalid                 = &errHTTP{40062, http.StatusBadRequest, "invalid request: tier code invalid, or tier name missing", "", nil, nil}
	errHTTPBadRequestTierReservationsLimit           = &errHTTP{40063, http.StatusBadRequest, "invalid request: user has more topic reservations than the tier allows, remove reservations first", "", nil, nil}
	errHTTPBadRequestAccessAdminUser                 = &errHTTP{40064, http.StatusBadRequest, "invalid request: admins can access all topics, access control entries are not allowed", "", nil, nil}
	errHTTPBadRequestTokenNotFound                   = &errHTTP{40065, http.StatusBadRequest, "invalid request: token does not exist", "", nil, nil}
	errHTTPBadRequestRoleInvalid                     = &errHTTP{40066, http.StatusBadRequest, "invalid request: role must be user or admin", "", nil, nil}
	errHTTPBadRequestUserNoPrimaryEmail              = &errHTTP{40067, http.StatusBadRequest, "invalid request: user has no primary email, cannot send reset link", "", nil, nil}
	errHTTPBadRequestBaseURLNotConfigured            = &errHTTP{40068, http.StatusBadRequest, "invalid request: base-url must be configured on the server to generate links", "https://ntfy.sh/docs/config/", nil, nil}
	errHTTPBadRequestPasswordHashInvalid             = &errHTTP{40069, http.StatusBadRequest, "invalid request: password hash must be a bcrypt hash with cost 10 or higher, use 'ntfy user hash' to generate", "", nil, nil}
	errHTTPNotFound                                  = &errHTTP{40401, http.StatusNotFound, "page not found", "", nil, nil}
	errHTTPUnauthorized                              = &errHTTP{40101, http.StatusUnauthorized, "unauthorized", "https://ntfy.sh/docs/publish/#authentication", nil, nil}
	errHTTPForbidden                                 = &errHTTP{40301, http.StatusForbidden, "forbidden", "https://ntfy.sh/docs/publish/#authentication", nil, nil}
	errHTTPForbiddenAdminSocketOnly                  = &errHTTP{40302, http.StatusForbidden, "forbidden: this operation is only allowed over the local admin socket", "https://ntfy.sh/docs/config/#admin-socket", nil, nil}
	errHTTPConflictUserExists                        = &errHTTP{40901, http.StatusConflict, "conflict: user already exists", "", nil, nil}
	errHTTPConflictTopicReserved                     = &errHTTP{40902, http.StatusConflict, "conflict: access control entry for topic or topic pattern already exists", "", nil, nil}
	errHTTPConflictSubscriptionExists                = &errHTTP{40903, http.StatusConflict, "conflict: topic subscription already exists", "", nil, nil}
	errHTTPConflictPhoneNumberExists                 = &errHTTP{40904, http.StatusConflict, "conflict: phone number already exists", "", nil, nil}
	errHTTPConflictProvisionedUserChange             = &errHTTP{40905, http.StatusConflict, "conflict: cannot change or delete provisioned user", "", nil, nil}
	errHTTPConflictProvisionedTokenChange            = &errHTTP{40906, http.StatusConflict, "conflict: cannot change or delete provisioned token", "", nil, nil}
	errHTTPConflictEmailExists                       = &errHTTP{40907, http.StatusConflict, "conflict: email address already exists", "", nil, nil}
	errHTTPConflictEmailPrimaryElsewhere             = &errHTTP{40908, http.StatusConflict, "conflict: email address is the primary email on another account", "", nil, nil}
	errHTTPConflictTierExists                        = &errHTTP{40909, http.StatusConflict, "conflict: tier already exists", "", nil, nil}
	errHTTPConflictTierInUse                         = &errHTTP{40910, http.StatusConflict, "conflict: tier is still assigned to users", "", nil, nil}
	errHTTPGonePhoneVerificationExpired              = &errHTTP{41001, http.StatusGone, "phone number verification expired or does not exist", "", nil, nil}
	errHTTPEntityTooLargeAttachment                  = &errHTTP{41301, http.StatusRequestEntityTooLarge, "attachment too large, or bandwidth limit reached", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPEntityTooLargeMatrixRequest               = &errHTTP{41302, http.StatusRequestEntityTooLarge, "Matrix request is larger than the max allowed length", "", nil, nil}
	errHTTPEntityTooLargeJSONBody                    = &errHTTP{41303, http.StatusRequestEntityTooLarge, "JSON body too large", "", nil, nil}
	errHTTPTooManyRequestsLimitRequests              = &errHTTP{42901, http.StatusTooManyRequests, "limit reached: too many requests", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPTooManyRequestsLimitEmails                = &errHTTP{42902, http.StatusTooManyRequests, "limit reached: too many emails", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPTooManyRequestsLimitSubscriptions         = &errHTTP{42903, http.StatusTooManyRequests, "limit reached: too many active subscriptions", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPTooManyRequestsLimitTotalTopics           = &errHTTP{42904, http.StatusTooManyRequests, "limit reached: the total number of topics on the server has been reached, please contact the admin", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPTooManyRequestsLimitAttachmentBandwidth   = &errHTTP{42905, http.StatusTooManyRequests, "limit reached: daily bandwidth reached", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPTooManyRequestsLimitAccountActions        = &errHTTP{42906, http.StatusTooManyRequests, "limit reached: too many account requests", "https://ntfy.sh/docs/publish/#limitations", nil, nil} // FIXME document limit
	errHTTPTooManyRequestsLimitReservations          = &errHTTP{42907, http.StatusTooManyRequests, "limit reached: too many topic reservations for this user", "", nil, nil}
	errHTTPTooManyRequestsLimitMessages              = &errHTTP{42908, http.StatusTooManyRequests, "limit reached: daily message quota reached", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPTooManyRequestsLimitAuthFailure           = &errHTTP{42909, http.StatusTooManyRequests, "limit reached: too many auth failures", "https://ntfy.sh/docs/publish/#limitations", nil, nil} // FIXME document limit
	errHTTPTooManyRequestsLimitCalls                 = &errHTTP{42910, http.StatusTooManyRequests, "limit reached: daily phone call quota reached", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPTooManyRequestsLimitTopicCreation         = &errHTTP{42911, http.StatusTooManyRequests, "limit reached: too many new topics, please wait", "https://ntfy.sh/docs/publish/#limitations", nil, nil}
	errHTTPInternalError                             = &errHTTP{50001, http.StatusInternalServerError, "internal server error", "", nil, nil}
	errHTTPInternalErrorInvalidPath                  = &errHTTP{50002, http.StatusInternalServerError, "internal server error: invalid path", "", nil, nil}
	errHTTPInternalErrorMissingBaseURL               = &errHTTP{50003, http.StatusInternalServerError, "internal server error: base-url must be be configured for this feature", "https://ntfy.sh/docs/config/", nil, nil}
	errHTTPInternalErrorWebPushUnableToPublish       = &errHTTP{50004, http.StatusInternalServerError, "internal server error: unable to publish web push message", "", nil, nil}
	errHTTPServiceUnavailableDraining                = &errHTTP{50301, http.StatusServiceUnavailable, "service unavailable: this node is shutting down, please reconnect", "", nil, nil}
	errHTTPInsufficientStorageUnifiedPush            = &errHTTP{50701, http.StatusInsufficientStorage, "cannot publish to UnifiedPush topic without previously active subscriber", "", nil, nil}
)
