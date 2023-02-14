package server

import (
	"encoding/json"
	"fmt"
	"heckel.io/ntfy/log"
	"net/http"
)

// errHTTP is a generic HTTP error for any non-200 HTTP error
type errHTTP struct {
	Code     int    `json:"code,omitempty"`
	HTTPCode int    `json:"http"`
	Message  string `json:"error"`
	Link     string `json:"link,omitempty"`
}

func (e errHTTP) Error() string {
	return e.Message
}

func (e errHTTP) JSON() string {
	b, _ := json.Marshal(&e)
	return string(b)
}

func (e errHTTP) Context() log.Context {
	return log.Context{
		"error":       e.Message,
		"error_code":  e.Code,
		"http_status": e.HTTPCode,
	}
}

func wrapErrHTTP(err *errHTTP, message string, args ...any) *errHTTP {
	return &errHTTP{
		Code:     err.Code,
		HTTPCode: err.HTTPCode,
		Message:  fmt.Sprintf("%s, %s", err.Message, fmt.Sprintf(message, args...)),
		Link:     err.Link,
	}
}

var (
	errHTTPBadRequest                                = &errHTTP{40000, http.StatusBadRequest, "invalid request", ""}
	errHTTPBadRequestEmailDisabled                   = &errHTTP{40001, http.StatusBadRequest, "e-mail notifications are not enabled", "https://ntfy.sh/docs/config/#e-mail-notifications"}
	errHTTPBadRequestDelayNoCache                    = &errHTTP{40002, http.StatusBadRequest, "cannot disable cache for delayed message", ""}
	errHTTPBadRequestDelayNoEmail                    = &errHTTP{40003, http.StatusBadRequest, "delayed e-mail notifications are not supported", ""}
	errHTTPBadRequestDelayCannotParse                = &errHTTP{40004, http.StatusBadRequest, "invalid delay parameter: unable to parse delay", "https://ntfy.sh/docs/publish/#scheduled-delivery"}
	errHTTPBadRequestDelayTooSmall                   = &errHTTP{40005, http.StatusBadRequest, "invalid delay parameter: too small, please refer to the docs", "https://ntfy.sh/docs/publish/#scheduled-delivery"}
	errHTTPBadRequestDelayTooLarge                   = &errHTTP{40006, http.StatusBadRequest, "invalid delay parameter: too large, please refer to the docs", "https://ntfy.sh/docs/publish/#scheduled-delivery"}
	errHTTPBadRequestPriorityInvalid                 = &errHTTP{40007, http.StatusBadRequest, "invalid priority parameter", "https://ntfy.sh/docs/publish/#message-priority"}
	errHTTPBadRequestSinceInvalid                    = &errHTTP{40008, http.StatusBadRequest, "invalid since parameter", "https://ntfy.sh/docs/subscribe/api/#fetch-cached-messages"}
	errHTTPBadRequestTopicInvalid                    = &errHTTP{40009, http.StatusBadRequest, "invalid request: topic invalid", ""}
	errHTTPBadRequestTopicDisallowed                 = &errHTTP{40010, http.StatusBadRequest, "invalid request: topic name is not allowed", ""}
	errHTTPBadRequestMessageNotUTF8                  = &errHTTP{40011, http.StatusBadRequest, "invalid message: message must be UTF-8 encoded", ""}
	errHTTPBadRequestAttachmentURLInvalid            = &errHTTP{40013, http.StatusBadRequest, "invalid request: attachment URL is invalid", "https://ntfy.sh/docs/publish/#attachments"}
	errHTTPBadRequestAttachmentsDisallowed           = &errHTTP{40014, http.StatusBadRequest, "invalid request: attachments not allowed", "https://ntfy.sh/docs/config/#attachments"}
	errHTTPBadRequestAttachmentsExpiryBeforeDelivery = &errHTTP{40015, http.StatusBadRequest, "invalid request: attachment expiry before delayed delivery date", "https://ntfy.sh/docs/publish/#scheduled-delivery"}
	errHTTPBadRequestWebSocketsUpgradeHeaderMissing  = &errHTTP{40016, http.StatusBadRequest, "invalid request: client not using the websocket protocol", "https://ntfy.sh/docs/subscribe/api/#websockets"}
	errHTTPBadRequestMessageJSONInvalid              = &errHTTP{40017, http.StatusBadRequest, "invalid request: request body must be message JSON", "https://ntfy.sh/docs/publish/#publish-as-json"}
	errHTTPBadRequestActionsInvalid                  = &errHTTP{40018, http.StatusBadRequest, "invalid request: actions invalid", "https://ntfy.sh/docs/publish/#action-buttons"}
	errHTTPBadRequestMatrixMessageInvalid            = &errHTTP{40019, http.StatusBadRequest, "invalid request: Matrix JSON invalid", "https://ntfy.sh/docs/publish/#matrix-gateway"}
	errHTTPBadRequestMatrixPushkeyBaseURLMismatch    = &errHTTP{40020, http.StatusBadRequest, "invalid request: push key must be prefixed with base URL", "https://ntfy.sh/docs/publish/#matrix-gateway"}
	errHTTPBadRequestIconURLInvalid                  = &errHTTP{40021, http.StatusBadRequest, "invalid request: icon URL is invalid", "https://ntfy.sh/docs/publish/#icons"}
	errHTTPBadRequestSignupNotEnabled                = &errHTTP{40022, http.StatusBadRequest, "invalid request: signup not enabled", "https://ntfy.sh/docs/config"}
	errHTTPBadRequestNoTokenProvided                 = &errHTTP{40023, http.StatusBadRequest, "invalid request: no token provided", ""}
	errHTTPBadRequestJSONInvalid                     = &errHTTP{40024, http.StatusBadRequest, "invalid request: request body must be valid JSON", ""}
	errHTTPBadRequestPermissionInvalid               = &errHTTP{40025, http.StatusBadRequest, "invalid request: incorrect permission string", ""}
	errHTTPBadRequestIncorrectPasswordConfirmation   = &errHTTP{40026, http.StatusBadRequest, "invalid request: password confirmation is not correct", ""}
	errHTTPBadRequestNotAPaidUser                    = &errHTTP{40027, http.StatusBadRequest, "invalid request: not a paid user", ""}
	errHTTPBadRequestBillingRequestInvalid           = &errHTTP{40028, http.StatusBadRequest, "invalid request: not a valid billing request", ""}
	errHTTPBadRequestBillingSubscriptionExists       = &errHTTP{40029, http.StatusBadRequest, "invalid request: billing subscription already exists", ""}
	errHTTPNotFound                                  = &errHTTP{40401, http.StatusNotFound, "page not found", ""}
	errHTTPUnauthorized                              = &errHTTP{40101, http.StatusUnauthorized, "unauthorized", "https://ntfy.sh/docs/publish/#authentication"}
	errHTTPForbidden                                 = &errHTTP{40301, http.StatusForbidden, "forbidden", "https://ntfy.sh/docs/publish/#authentication"}
	errHTTPConflictUserExists                        = &errHTTP{40901, http.StatusConflict, "conflict: user already exists", ""}
	errHTTPConflictTopicReserved                     = &errHTTP{40902, http.StatusConflict, "conflict: access control entry for topic or topic pattern already exists", ""}
	errHTTPConflictSubscriptionExists                = &errHTTP{40903, http.StatusConflict, "conflict: topic subscription already exists", ""}
	errHTTPEntityTooLargeAttachment                  = &errHTTP{41301, http.StatusRequestEntityTooLarge, "attachment too large, or bandwidth limit reached", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPEntityTooLargeMatrixRequest               = &errHTTP{41302, http.StatusRequestEntityTooLarge, "Matrix request is larger than the max allowed length", ""}
	errHTTPEntityTooLargeJSONBody                    = &errHTTP{41303, http.StatusRequestEntityTooLarge, "JSON body too large", ""}
	errHTTPTooManyRequestsLimitRequests              = &errHTTP{42901, http.StatusTooManyRequests, "limit reached: too many requests, please be nice", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitEmails                = &errHTTP{42902, http.StatusTooManyRequests, "limit reached: too many emails, please be nice", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitSubscriptions         = &errHTTP{42903, http.StatusTooManyRequests, "limit reached: too many active subscriptions, please be nice", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitTotalTopics           = &errHTTP{42904, http.StatusTooManyRequests, "limit reached: the total number of topics on the server has been reached, please contact the admin", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitAttachmentBandwidth   = &errHTTP{42905, http.StatusTooManyRequests, "limit reached: daily bandwidth reached", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitAccountCreation       = &errHTTP{42906, http.StatusTooManyRequests, "limit reached: too many accounts created", "https://ntfy.sh/docs/publish/#limitations"} // FIXME document limit
	errHTTPTooManyRequestsLimitReservations          = &errHTTP{42907, http.StatusTooManyRequests, "limit reached: too many topic reservations for this user", ""}
	errHTTPTooManyRequestsLimitMessages              = &errHTTP{42908, http.StatusTooManyRequests, "limit reached: daily message quota reached", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitAuthFailure           = &errHTTP{42909, http.StatusTooManyRequests, "limit reached: too many auth failures", "https://ntfy.sh/docs/publish/#limitations"} // FIXME document limit
	errHTTPInternalError                             = &errHTTP{50001, http.StatusInternalServerError, "internal server error", ""}
	errHTTPInternalErrorInvalidPath                  = &errHTTP{50002, http.StatusInternalServerError, "internal server error: invalid path", ""}
	errHTTPInternalErrorMissingBaseURL               = &errHTTP{50003, http.StatusInternalServerError, "internal server error: base-url must be be configured for this feature", "https://ntfy.sh/docs/config/"}
	errHTTPWontStoreMessage                          = &errHTTP{50701, http.StatusInsufficientStorage, "topic is inactive; no device available to recieve message", ""}
)
