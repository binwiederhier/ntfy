package client

import (
	"net/http"
)

type PublishOption func(r *http.Request) error

func WithTitle(title string) PublishOption {
	return func(r *http.Request) error {
		if title != "" {
			r.Header.Set("X-Title", title)
		}
		return nil
	}
}

func WithPriority(priority string) PublishOption {
	return func(r *http.Request) error {
		if priority != "" {
			r.Header.Set("X-Priority", priority)
		}
		return nil
	}
}

func WithTags(tags string) PublishOption {
	return func(r *http.Request) error {
		if tags != "" {
			r.Header.Set("X-Tags", tags)
		}
		return nil
	}
}

func WithDelay(delay string) PublishOption {
	return func(r *http.Request) error {
		if delay != "" {
			r.Header.Set("X-Delay", delay)
		}
		return nil
	}
}

func WithNoCache() PublishOption {
	return WithHeader("X-Cache", "no")
}

func WithNoFirebase() PublishOption {
	return WithHeader("X-Firebase", "no")
}

func WithHeader(header, value string) PublishOption {
	return func(r *http.Request) error {
		r.Header.Set(header, value)
		return nil
	}
}

type SubscribeOption func(r *http.Request) error

func WithSince(since string) PublishOption {
	return func(r *http.Request) error {
		if since != "" {
			q := r.URL.Query()
			q.Add("since", since)
			r.URL.RawQuery = q.Encode()
		}
		return nil
	}
}
