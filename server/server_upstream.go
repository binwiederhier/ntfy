package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

const (
	upstreamRetryLimit        = 100
	upstreamRetryInitialDelay = 5 * time.Second
	upstreamRetryMaxDelay     = time.Minute
)

type upstreamForwarder struct {
	config  *Config
	client  *http.Client
	ctx     context.Context
	cancel  context.CancelFunc
	retries chan struct{}
}

func newUpstreamForwarder(config *Config) *upstreamForwarder {
	ctx, cancel := context.WithCancel(context.Background())
	return &upstreamForwarder{
		config:  config,
		client:  &http.Client{Timeout: 10 * time.Second},
		ctx:     ctx,
		cancel:  cancel,
		retries: make(chan struct{}, upstreamRetryLimit),
	}
}

func (s *Server) forwardPollRequest(v *visitor, m *model.Message) {
	s.upstream.forward(m, logvm(v, m))
}

func (f *upstreamForwarder) forward(m *model.Message, ev *log.Event) {
	ctx := f.ctx
	deadline := time.Now().Add(f.config.UpstreamRetryDuration)
	expires := time.Unix(m.Expires, 0)
	retryEnabled := f.config.UpstreamRetryDuration > 0 && time.Now().Before(expires)
	if retryEnabled {
		if expires.Before(deadline) {
			deadline = expires
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	retry, retryAfter, err := f.send(ctx, m)
	if err == nil || f.ctx.Err() != nil {
		return
	}
	ev.Err(err).Warn("Unable to publish poll request")
	if !retryEnabled || !retry || ctx.Err() != nil {
		return
	}
	select {
	case f.retries <- struct{}{}:
		defer func() { <-f.retries }()
	default:
		ev.Warn("Upstream poll retry limit reached, dropping poll request")
		return
	}
	for backoff := upstreamRetryInitialDelay; ; backoff = min(backoff*2, upstreamRetryMaxDelay) {
		delay := max(backoff/2+time.Duration(rand.Int64N(int64(backoff/2))), retryAfter)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			return
		}
		retry, retryAfter, err = f.send(ctx, m)
		if err == nil || f.ctx.Err() != nil {
			return
		}
		ev.Err(err).Warn("Unable to publish poll request")
		if !retry || ctx.Err() != nil {
			return
		}
	}
}

func (f *upstreamForwarder) send(ctx context.Context, m *model.Message) (retry bool, retryAfter time.Duration, err error) {
	topicURL := fmt.Sprintf("%s/%s", f.config.BaseURL, m.Topic)
	topicHash := fmt.Sprintf("%x", sha256.Sum256([]byte(topicURL)))
	forwardURL := fmt.Sprintf("%s/%s", f.config.UpstreamBaseURL, topicHash)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, forwardURL, nil)
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("User-Agent", "ntfy/"+f.config.BuildVersion)
	req.Header.Set("X-Poll-ID", m.ID)
	if f.config.UpstreamAccessToken != "" {
		req.Header.Set("Authorization", util.BearerAuth(f.config.UpstreamAccessToken))
	}
	response, err := f.client.Do(req)
	if err != nil {
		return true, 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode == http.StatusOK {
		return false, 0, nil
	}
	retry = response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 && response.StatusCode <= 599
	retryAfter = upstreamRetryAfter(response.Header.Get("Retry-After"))
	if response.StatusCode == http.StatusTooManyRequests {
		return retry, retryAfter, fmt.Errorf("upstream server %s responded with HTTP %s; send fewer daily messages or configure upstream-access-token for an account with higher rate limits", f.config.UpstreamBaseURL, response.Status)
	}
	return retry, retryAfter, fmt.Errorf("upstream server %s responded with HTTP %s", f.config.UpstreamBaseURL, response.Status)
}

func upstreamRetryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(time.Until(date), 0)
	}
	return 0
}
