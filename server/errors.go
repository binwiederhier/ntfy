package server

import (
	"encoding/json"
	"fmt"
	"heckel.io/ntfy/v2/log"
	"net/http"
)

// errHTTP is a generic HTTP error for any non-200 HTTP error
type errHTTP struct {
	Code     int    `json:"code,omitempty"`
	HTTPCode int    `json:"http"`
	Message  string `json:"error"`
	Link     string `json:"link,omitempty"`
	context  log.Context
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
	}
}

var (
	errHTTPBadRequest                                = &errHTTP{40000, http.StatusBadRequest, "invalid request", "", nil}
	errHTTPBadRequestEmailDisabled                   = &errHTTP{40001, http.StatusBadRequest, "e-mail notifications are not enabled", "https://ntfy.sh/docs/config/#e-mail-notifications", nil}
	errHTTPBadRequestDelayNoCache                    = &errHTTP{40002, http.StatusBadRequest, "cannot disable cache for delayed message", "", nil}
	errHTTPBadRequestDelayNoEmail                    = &errHTTP{40003, http.StatusBadRequest, "delayed e-mail notifications are not supported", "", nil}
	errHTTPBadRequestDelayCannotParse                = &errHTTP{40004, http.StatusBadRequest, "invalid delay parameter: unable to parse delay", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil}
	errHTTPBadRequestDelayTooSmall                   = &errHTTP{40005, http.StatusBadRequest, "invalid delay parameter: too small, please refer to the docs", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil}
	errHTTPBadRequestDelayTooLarge                   = &errHTTP{40006, http.StatusBadRequest, "invalid delay parameter: too large, please refer to the docs", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil}
	errHTTPBadRequestPriorityInvalid                 = &errHTTP{40007, http.StatusBadRequest, "invalid priority parameter", "https://ntfy.sh/docs/publish/#message-priority", nil}
	errHTTPBadRequestSinceInvalid                    = &errHTTP{40008, http.StatusBadRequest, "invalid since parameter", "https://ntfy.sh/docs/subscribe/api/#fetch-cached-messages", nil}
	errHTTPBadRequestTopicInvalid                    = &errHTTP{40009, http.StatusBadRequest, "invalid request: topic invalid", "", nil}
	errHTTPBadRequestTopicDisallowed                 = &errHTTP{40010, http.StatusBadRequest, "invalid request: topic name is not allowed", "", nil}
	errHTTPBadRequestMessageNotUTF8                  = &errHTTP{40011, http.StatusBadRequest, "invalid message: message must be UTF-8 encoded", "", nil}
	errHTTPBadRequestAttachmentURLInvalid            = &errHTTP{40013, http.StatusBadRequest, "invalid request: attachment URL is invalid", "https://ntfy.sh/docs/publish/#attachments", nil}
	errHTTPBadRequestAttachmentsDisallowed           = &errHTTP{40014, http.StatusBadRequest, "invalid request: attachments not allowed", "https://ntfy.sh/docs/config/#attachments", nil}
	errHTTPBadRequestAttachmentsExpiryBeforeDelivery = &errHTTP{40015, http.StatusBadRequest, "invalid request: attachment expiry before delayed delivery date", "https://ntfy.sh/docs/publish/#scheduled-delivery", nil}
	errHTTPBadRequestWebSocketsUpgradeHeaderMissing  = &errHTTP{40016, http.StatusBadRequest, "invalid request: client not using the websocket protocol", "https://ntfy.sh/docs/subscribe/api/#websockets", nil}
	errHTTPBadRequestMessageJSONInvalid              = &errHTTP{40017, http.StatusBadRequest, "invalid request: request body must be message JSON", "https://ntfy.sh/docs/publish/#publish-as-json", nil}
	errHTTPBadRequestActionsInvalid                  = &errHTTP{40018, http.StatusBadRequest, "invalid request: actions invalid", "https://ntfy.sh/docs/publish/#action-buttons", nil}
	errHTTPBadRequestMatrixMessageInvalid            = &errHTTP{40019, http.StatusBadRequest, "invalid request: Matrix JSON invalid", "https://ntfy.sh/docs/publish/#matrix-gateway", nil}
	errHTTPBadRequestIconURLInvalid                  = &errHTTP{40021, http.StatusBadRequest, "invalid request: icon URL is invalid", "https://ntfy.sh/docs/publish/#icons", nil}
	errHTTPBadRequestSignupNotEnabled                = &errHTTP{40022, http.StatusBadRequest, "invalid request: signup not enabled", "https://ntfy.sh/docs/config", nil}
	errHTTPBadRequestNoTokenProvided                 = &errHTTP{40023, http.StatusBadRequest, "invalid request: no token provided", "", nil}
	errHTTPBadRequestJSONInvalid                     = &errHTTP{40024, http.StatusBadRequest, "invalid request: request body must be valid JSON", "", nil}
	errHTTPBadRequestPermissionInvalid               = &errHTTP{40025, http.StatusBadRequest, "invalid request: incorrect permission string", "", nil}
	errHTTPBadRequestIncorrectPasswordConfirmation   = &errHTTP{40026, http.StatusBadRequest, "invalid request: password confirmation is not correct", "", nil}
	errHTTPBadRequestNotAPaidUser                    = &errHTTP{40027, http.StatusBadRequest, "invalid request: not a paid user", "", nil}
	errHTTPBadRequestBillingRequestInvalid           = &errHTTP{40028, http.StatusBadRequest, "invalid request: not a valid billing request", "", nil}
	errHTTPBadRequestBillingSubscriptionExists       = &errHTTP{40029, http.StatusBadRequest, "invalid request: billing subscription already exists", "", nil}
	errHTTPBadRequestTierInvalid                     = &errHTTP{40030, http.StatusBadRequest, "invalid request: tier does not exist", "", nil}
	errHTTPBadRequestUserNotFound                    = &errHTTP{40031, http.StatusBadRequest, "invalid request: user does not exist", "", nil}
	errHTTPBadRequestPhoneCallsDisabled              = &errHTTP{40032, http.StatusBadRequest, "invalid request: calling is disabled", "https://ntfy.sh/docs/config/#phone-calls", nil}
	errHTTPBadRequestPhoneNumberInvalid              = &errHTTP{40033, http.StatusBadRequest, "invalid request: phone number invalid", "https://ntfy.sh/docs/publish/#phone-calls", nil}
	errHTTPBadRequestPhoneNumberNotVerified          = &errHTTP{40034, http.StatusBadRequest, "invalid request: phone number not verified, or no matching verified numbers found", "https://ntfy.sh/docs/publish/#phone-calls", nil}
	errHTTPBadRequestAnonymousCallsNotAllowed        = &errHTTP{40035, http.StatusBadRequest, "invalid request: anonymous phone calls are not allowed", "https://ntfy.sh/docs/publish/#phone-calls", nil}
	errHTTPBadRequestPhoneNumberVerifyChannelInvalid = &errHTTP{40036, http.StatusBadRequest, "invalid request: verification channel must be 'sms' or 'call'", "https://ntfy.sh/docs/publish/#phone-calls", nil}
	errHTTPBadRequestDelayNoCall                     = &errHTTP{40037, http.StatusBadRequest, "delayed call notifications are not supported", "", nil}
	errHTTPBadRequestWebPushSubscriptionInvalid      = &errHTTP{40038, http.StatusBadRequest, "invalid request: web push payload malformed", "", nil}
	errHTTPBadRequestWebPushEndpointUnknown          = &errHTTP{40039, http.StatusBadRequest, "invalid request: web push endpoint unknown", "", nil}
	errHTTPBadRequestWebPushTopicCountTooHigh        = &errHTTP{40040, http.StatusBadRequest, "invalid request: too many web push topic subscriptions", "", nil}
	errHTTPNotFound                                  = &errHTTP{40401, http.StatusNotFound, "page not found", "", nil}
	errHTTPUnauthorized                              = &errHTTP{40101, http.StatusUnauthorized, "unauthorized", "https://ntfy.sh/docs/publish/#authentication", nil}
	errHTTPForbidden                                 = &errHTTP{40301, http.StatusForbidden, "forbidden", "https://ntfy.sh/docs/publish/#authentication", nil}
	errHTTPConflictUserExists                        = &errHTTP{40901, http.StatusConflict, "conflict: user already exists", "", nil}
	errHTTPConflictTopicReserved                     = &errHTTP{40902, http.StatusConflict, "conflict: access control entry for topic or topic pattern already exists", "", nil}
	errHTTPConflictSubscriptionExists                = &errHTTP{40903, http.StatusConflict, "conflict: topic subscription already exists", "", nil}
	errHTTPConflictPhoneNumberExists                 = &errHTTP{40904, http.StatusConflict, "conflict: phone number already exists", "", nil}
	errHTTPGonePhoneVerificationExpired              = &errHTTP{41001, http.StatusGone, "phone number verification expired or does not exist", "", nil}
	errHTTPEntityTooLargeAttachment                  = &errHTTP{41301, http.StatusRequestEntityTooLarge, "attachment too large, or bandwidth limit reached", "https://ntfy.sh/docs/publish/#limitations", nil}
	errHTTPEntityTooLargeMatrixRequest               = &errHTTP{41302, http.StatusRequestEntityTooLarge, "Matrix request is larger than the max allowed length", "", nil}
	errHTTPEntityTooLargeJSONBody                    = &errHTTP{41303, http.StatusRequestEntityTooLarge, "JSON body too large", "", nil}
	errHTTPTooManyRequestsLimitRequests              = &errHTTP{42901, http.StatusTooManyRequests, "limit reached: too many requests", "https://ntfy.sh/docs/publish/#limitations", nil}
	errHTTPTooManyRequestsLimitEmails                = &errHTTP{42902, http.StatusTooManyRequests, "limit reached: too many emails", "https://ntfy.sh/docs/publish/#limitations", nil}
	errHTTPTooManyRequestsLimitSubscriptions         = &errHTTP{42903, http.StatusTooManyRequests, "limit reached: too many active subscriptions", "https://ntfy.sh/docs/publish/#limitations", nil}
	errHTTPTooManyRequestsLimitTotalTopics           = &errHTTP{42904, http.StatusTooManyRequests, "limit reached: the total number of topics on the server has been reached, please contact the admin", "https://ntfy.sh/docs/publish/#limitations", nil}
	errHTTPTooManyRequestsLimitAttachmentBandwidth   = &errHTTP{42905, http.StatusTooManyRequests, "limit reached: daily bandwidth reached", "https://ntfy.sh/docs/publish/#limitations", nil}
	errHTTPTooManyRequestsLimitAccountCreation       = &errHTTP{42906, http.StatusTooManyRequests, "limit reached: too many accounts created", "https://ntfy.sh/docs/publish/#limitations", nil} // FIXME document limit
	errHTTPTooManyRequestsLimitReservations          = &errHTTP{42907, http.StatusTooManyRequests, "limit reached: too many topic reservations for this user", "", nil}
	errHTTPTooManyRequestsLimitMessages              = &errHTTP{42908, http.StatusTooManyRequests, "limit reached: daily message quota reached", "https://ntfy.sh/docs/publish/#limitations", nil}
	errHTTPTooManyRequestsLimitAuthFailure           = &errHTTP{42909, http.StatusTooManyRequests, "limit reached: too many auth failures", "https://ntfy.sh/docs/publish/#limitations", nil} // FIXME document limit
	errHTTPTooManyRequestsLimitCalls                 = &errHTTP{42910, http.StatusTooManyRequests, "limit reached: daily phone call quota reached", "https://ntfy.sh/docs/publish/#limitations", nil}
	errHTTPInternalError                             = &errHTTP{50001, http.StatusInternalServerError, "internal server error", "", nil}
	errHTTPInternalErrorInvalidPath                  = &errHTTP{50002, http.StatusInternalServerError, "internal server error: invalid path", "", nil}
	errHTTPInternalErrorMissingBaseURL               = &errHTTP{50003, http.StatusInternalServerError, "internal server error: base-url must be be configured for this feature", "https://ntfy.sh/docs/config/", nil}
	errHTTPInternalErrorWebPushUnableToPublish       = &errHTTP{50004, http.StatusInternalServerError, "internal server error: unable to publish web push message", "", nil}
	errHTTPInsufficientStorageUnifiedPush            = &errHTTP{50701, http.StatusInsufficientStorage, "cannot publish to UnifiedPush topic without previously active subscriber", "", nil}
)
