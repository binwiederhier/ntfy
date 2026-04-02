// Event types for ntfy messages
// These correspond to the server event types in server/types.go

export const EVENT_OPEN = "open";
export const EVENT_KEEPALIVE = "keepalive";
export const EVENT_MESSAGE = "message";
export const EVENT_MESSAGE_DELETE = "message_delete";
export const EVENT_MESSAGE_CLEAR = "message_clear";
export const EVENT_POLL_REQUEST = "poll_request";

export const WEBPUSH_EVENT_MESSAGE = "message";
export const WEBPUSH_EVENT_SUBSCRIPTION_EXPIRING = "subscription_expiring";

// Check if an event is a notification event (message, delete, or read)
export const isNotificationEvent = (event) => event === EVENT_MESSAGE || event === EVENT_MESSAGE_DELETE || event === EVENT_MESSAGE_CLEAR;
