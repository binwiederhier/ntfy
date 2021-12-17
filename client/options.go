package client

import (
	"net/http"
)

type RequestOption func(r *http.Request) error
type PublishOption = RequestOption
type SubscribeOption = RequestOption

func WithTitle(title string) PublishOption {
	return WithHeader("X-Title", title)
}

func WithPriority(priority string) PublishOption {
	return WithHeader("X-Priority", priority)
}

func WithTags(tags string) PublishOption {
	return WithHeader("X-Tags", tags)
}

func WithDelay(delay string) PublishOption {
	return WithHeader("X-Delay", delay)
}

func WithNoCache() PublishOption {
	return WithHeader("X-Cache", "no")
}

func WithNoFirebase() PublishOption {
	return WithHeader("X-Firebase", "no")
}

func WithSince(since string) SubscribeOption {
	return WithQueryParam("since", since)
}

func WithPoll() SubscribeOption {
	return WithQueryParam("poll", "1")
}

func WithScheduled() SubscribeOption {
	return WithQueryParam("scheduled", "1")
}

func WithHeader(header, value string) RequestOption {
	return func(r *http.Request) error {
		if value != "" {
			r.Header.Set(header, value)
		}
		return nil
	}
}

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
