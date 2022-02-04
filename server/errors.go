package server

import (
	"encoding/json"
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

var (
	errHTTPBadRequestEmailDisabled                   = &errHTTP{40001, http.StatusBadRequest, "e-mail notifications are not enabled", "https://ntfy.sh/docs/config/#e-mail-notifications"}
	errHTTPBadRequestDelayNoCache                    = &errHTTP{40002, http.StatusBadRequest, "cannot disable cache for delayed message", ""}
	errHTTPBadRequestDelayNoEmail                    = &errHTTP{40003, http.StatusBadRequest, "delayed e-mail notifications are not supported", ""}
	errHTTPBadRequestDelayCannotParse                = &errHTTP{40004, http.StatusBadRequest, "invalid delay parameter: unable to parse delay", "https://ntfy.sh/docs/publish/#scheduled-delivery"}
	errHTTPBadRequestDelayTooSmall                   = &errHTTP{40005, http.StatusBadRequest, "invalid delay parameter: too small, please refer to the docs", "https://ntfy.sh/docs/publish/#scheduled-delivery"}
	errHTTPBadRequestDelayTooLarge                   = &errHTTP{40006, http.StatusBadRequest, "invalid delay parameter: too large, please refer to the docs", "https://ntfy.sh/docs/publish/#scheduled-delivery"}
	errHTTPBadRequestPriorityInvalid                 = &errHTTP{40007, http.StatusBadRequest, "invalid priority parameter", "https://ntfy.sh/docs/publish/#message-priority"}
	errHTTPBadRequestSinceInvalid                    = &errHTTP{40008, http.StatusBadRequest, "invalid since parameter", "https://ntfy.sh/docs/subscribe/api/#fetch-cached-messages"}
	errHTTPBadRequestTopicInvalid                    = &errHTTP{40009, http.StatusBadRequest, "invalid topic: path invalid", ""}
	errHTTPBadRequestTopicDisallowed                 = &errHTTP{40010, http.StatusBadRequest, "invalid topic: topic name is disallowed", ""}
	errHTTPBadRequestMessageNotUTF8                  = &errHTTP{40011, http.StatusBadRequest, "invalid message: message must be UTF-8 encoded", ""}
	errHTTPBadRequestAttachmentTooLarge              = &errHTTP{40012, http.StatusBadRequest, "invalid request: attachment too large, or bandwidth limit reached", ""}
	errHTTPBadRequestAttachmentURLInvalid            = &errHTTP{40013, http.StatusBadRequest, "invalid request: attachment URL is invalid", ""}
	errHTTPBadRequestAttachmentsDisallowed           = &errHTTP{40014, http.StatusBadRequest, "invalid request: attachments not allowed", ""}
	errHTTPBadRequestAttachmentsExpiryBeforeDelivery = &errHTTP{40015, http.StatusBadRequest, "invalid request: attachment expiry before delayed delivery date", ""}
	errHTTPBadRequestWebSocketsUpgradeHeaderMissing  = &errHTTP{40016, http.StatusBadRequest, "invalid request: client not using the websocket protocol", ""}
	errHTTPNotFound                                  = &errHTTP{40401, http.StatusNotFound, "page not found", ""}
	errHTTPUnauthorized                              = &errHTTP{40101, http.StatusUnauthorized, "unauthorized", "https://ntfy.sh/docs/publish/#authentication"}
	errHTTPForbidden                                 = &errHTTP{40301, http.StatusForbidden, "forbidden", "https://ntfy.sh/docs/publish/#authentication"}
	errHTTPTooManyRequestsLimitRequests              = &errHTTP{42901, http.StatusTooManyRequests, "limit reached: too many requests, please be nice", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitEmails                = &errHTTP{42902, http.StatusTooManyRequests, "limit reached: too many emails, please be nice", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitSubscriptions         = &errHTTP{42903, http.StatusTooManyRequests, "limit reached: too many active subscriptions, please be nice", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsLimitTotalTopics           = &errHTTP{42904, http.StatusTooManyRequests, "limit reached: the total number of topics on the server has been reached, please contact the admin", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPTooManyRequestsAttachmentBandwidthLimit   = &errHTTP{42905, http.StatusTooManyRequests, "too many requests: daily bandwidth limit reached", "https://ntfy.sh/docs/publish/#limitations"}
	errHTTPInternalError                             = &errHTTP{50001, http.StatusInternalServerError, "internal server error", ""}
	errHTTPInternalErrorInvalidFilePath              = &errHTTP{50002, http.StatusInternalServerError, "internal server error: invalid file path", ""}
)
