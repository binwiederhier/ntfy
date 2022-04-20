package client

import (
	"fmt"
	"heckel.io/ntfy/util"
	"net/http"
	"strings"
	"time"
)

// RequestOption is a generic request option that can be added to Client calls
type RequestOption = func(r *http.Request) error

// PublishOption is an option that can be passed to the Client.Publish call
type PublishOption = RequestOption

// SubscribeOption is an option that can be passed to a Client.Subscribe or Client.Poll call
type SubscribeOption = RequestOption

// WithMessage sets the notification message. This is an alternative way to passing the message body.
func WithMessage(message string) PublishOption {
	return WithHeader("X-Message", message)
}

// WithTitle adds a title to a message
func WithTitle(title string) PublishOption {
	return WithHeader("X-Title", title)
}

// WithPriority adds a priority to a message. The priority can be either a number (1=min, 5=max),
// or the corresponding names (see util.ParsePriority).
func WithPriority(priority string) PublishOption {
	return WithHeader("X-Priority", priority)
}

// WithTagsList adds a list of tags to a message. The tags parameter must be a comma-separated list
// of tags. To use a slice, use WithTags instead
func WithTagsList(tags string) PublishOption {
	return WithHeader("X-Tags", tags)
}

// WithTags adds a list of a tags to a message
func WithTags(tags []string) PublishOption {
	return WithTagsList(strings.Join(tags, ","))
}

// WithDelay instructs the server to send the message at a later date. The delay parameter can be a
// Unix timestamp, a duration string or a natural langage string. See https://ntfy.sh/docs/publish/#scheduled-delivery
// for details.
func WithDelay(delay string) PublishOption {
	return WithHeader("X-Delay", delay)
}

// WithClick makes the notification action open the given URL as opposed to entering the detail view
func WithClick(url string) PublishOption {
	return WithHeader("X-Click", url)
}

// WithActions adds custom user actions to the notification. The value can be either a JSON array or the
// simple format definition. See https://ntfy.sh/docs/publish/#action-buttons for details.
func WithActions(value string) PublishOption {
	return WithHeader("X-Actions", value)
}

// WithAttach sets a URL that will be used by the client to download an attachment
func WithAttach(attach string) PublishOption {
	return WithHeader("X-Attach", attach)
}

// WithFilename sets a filename for the attachment, and/or forces the HTTP body to interpreted as an attachment
func WithFilename(filename string) PublishOption {
	return WithHeader("X-Filename", filename)
}

// WithEmail instructs the server to also send the message to the given e-mail address
func WithEmail(email string) PublishOption {
	return WithHeader("X-Email", email)
}

// WithBasicAuth adds the Authorization header for basic auth to the request
func WithBasicAuth(user, pass string) PublishOption {
	return WithHeader("Authorization", util.BasicAuth(user, pass))
}

// WithNoCache instructs the server not to cache the message server-side
func WithNoCache() PublishOption {
	return WithHeader("X-Cache", "no")
}

// WithNoFirebase instructs the server not to forward the message to Firebase
func WithNoFirebase() PublishOption {
	return WithHeader("X-Firebase", "no")
}

// WithSince limits the number of messages returned from the server. The parameter since can be a Unix
// timestamp (see WithSinceUnixTime), a duration (WithSinceDuration) the word "all" (see WithSinceAll).
func WithSince(since string) SubscribeOption {
	return WithQueryParam("since", since)
}

// WithSinceAll instructs the server to return all messages for the given topic from the server
func WithSinceAll() SubscribeOption {
	return WithSince("all")
}

// WithSinceDuration instructs the server to return all messages since the given duration ago
func WithSinceDuration(since time.Duration) SubscribeOption {
	return WithSinceUnixTime(time.Now().Add(-1 * since).Unix())
}

// WithSinceUnixTime instructs the server to return only messages newer or equal to the given timestamp
func WithSinceUnixTime(since int64) SubscribeOption {
	return WithSince(fmt.Sprintf("%d", since))
}

// WithPoll instructs the server to close the connection after messages have been returned. Don't use this option
// directly. Use Client.Poll instead.
func WithPoll() SubscribeOption {
	return WithQueryParam("poll", "1")
}

// WithScheduled instructs the server to also return messages that have not been sent yet, i.e. delayed/scheduled
// messages (see WithDelay). The messages will have a future date.
func WithScheduled() SubscribeOption {
	return WithQueryParam("scheduled", "1")
}

// WithFilter is a generic subscribe option meant to be used to filter for certain messages only
func WithFilter(param, value string) SubscribeOption {
	return WithQueryParam(param, value)
}

// WithMessageFilter instructs the server to only return messages that match the exact message
func WithMessageFilter(message string) SubscribeOption {
	return WithQueryParam("message", message)
}

// WithTitleFilter instructs the server to only return messages with a title that match the exact string
func WithTitleFilter(title string) SubscribeOption {
	return WithQueryParam("title", title)
}

// WithPriorityFilter instructs the server to only return messages with the matching priority. Not that messages
// without priority also implicitly match priority 3.
func WithPriorityFilter(priority int) SubscribeOption {
	return WithQueryParam("priority", fmt.Sprintf("%d", priority))
}

// WithTagsFilter instructs the server to only return messages that contain all of the given tags
func WithTagsFilter(tags []string) SubscribeOption {
	return WithQueryParam("tags", strings.Join(tags, ","))
}

// WithHeader is a generic option to add headers to a request
func WithHeader(header, value string) RequestOption {
	return func(r *http.Request) error {
		if value != "" {
			r.Header.Set(header, value)
		}
		return nil
	}
}

// WithQueryParam is a generic option to add query parameters to a request
func WithQueryParam(param, value string) RequestOption {
	return func(r *http.Request) error {
		if value != "" {
			q := r.URL.Query()
			q.Add(param, value)
			r.URL.RawQuery = q.Encode()
		}
		return nil
	}
}
