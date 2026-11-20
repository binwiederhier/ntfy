package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
)

type upstreamRoundTripper func(*http.Request) (*http.Response, error)

func (f upstreamRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type upstreamResponseBody struct {
	io.Reader
	closed bool
}

func (b *upstreamResponseBody) Close() error { b.closed = true; return nil }

func newTestUpstreamForwarder(t *testing.T) (*upstreamForwarder, *model.Message) {
	t.Helper()
	conf := NewConfig()
	conf.BaseURL = "http://myserver.internal"
	conf.UpstreamBaseURL = "https://upstream.example"
	conf.UpstreamAccessToken = "tk_test"
	conf.BuildVersion = "test"
	conf.UpstreamRetryDuration = 15 * time.Minute
	f := newUpstreamForwarder(conf)
	t.Cleanup(f.cancel)
	return f, &model.Message{ID: "poll-id", Topic: "mytopic", Expires: time.Now().Add(time.Hour).Unix()}
}

func TestUpstreamForwarder_Retry(t *testing.T) {
	for _, status := range []int{0, http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, m := newTestUpstreamForwarder(t)
				var attempts []time.Time
				var bodies []*upstreamResponseBody
				f.client.Transport = upstreamRoundTripper(func(r *http.Request) (*http.Response, error) {
					require.Equal(t, "https://upstream.example/87c9cddf7b0105f5fe849bf084c6e600be0fde99be3223335199b4965bd7b735", r.URL.String())
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, m.ID, r.Header.Get("X-Poll-ID"))
					require.Equal(t, "Bearer tk_test", r.Header.Get("Authorization"))
					require.Equal(t, "ntfy/test", r.Header.Get("User-Agent"))
					require.Zero(t, r.ContentLength)
					attempts = append(attempts, time.Now())
					code := status
					if len(attempts) == 3 {
						code = http.StatusOK
					}
					if code == 0 {
						return nil, errors.New("network is unreachable")
					}
					body := &upstreamResponseBody{Reader: strings.NewReader("response")}
					bodies = append(bodies, body)
					return &http.Response{StatusCode: code, Status: http.StatusText(code), Body: body, Header: make(http.Header)}, nil
				})
				f.forward(m, log.With(m))
				require.Len(t, attempts, 3)
				require.InDelta(t, 3.75, attempts[1].Sub(attempts[0]).Seconds(), 1.25)
				require.InDelta(t, 7.5, attempts[2].Sub(attempts[1]).Seconds(), 2.5)
				for _, body := range bodies {
					require.True(t, body.closed)
				}
				require.Empty(t, f.retries)
			})
		})
	}
}

func TestUpstreamForwarder_NoRetry(t *testing.T) {
	for _, mode := range []string{"disabled", "uncached", "expired", "unauthorized", "forbidden", "bad-request", "full"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, m := newTestUpstreamForwarder(t)
				status := http.StatusServiceUnavailable
				switch mode {
				case "disabled":
					f.config.UpstreamRetryDuration = 0
				case "uncached":
					m.Expires = 0
				case "expired":
					m.Expires = time.Now().Add(-time.Second).Unix()
				case "unauthorized":
					status = http.StatusUnauthorized
				case "forbidden":
					status = http.StatusForbidden
				case "bad-request":
					status = http.StatusBadRequest
				case "full":
					for range cap(f.retries) {
						f.retries <- struct{}{}
					}
				}
				calls := 0
				f.client.Transport = upstreamRoundTripper(func(r *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: http.NoBody, Header: make(http.Header)}, nil
				})
				start := time.Now()
				f.forward(m, log.With(m))
				require.Equal(t, 1, calls)
				require.Equal(t, start, time.Now())
			})
		})
	}
}

func TestUpstreamForwarder_Deadline(t *testing.T) {
	for _, mode := range []string{"duration", "expiry", "retry-after", "in-flight"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, m := newTestUpstreamForwarder(t)
				limit := 8 * time.Second
				f.config.UpstreamRetryDuration = limit
				if mode == "expiry" {
					f.config.UpstreamRetryDuration = time.Hour
					m.Expires = time.Now().Add(limit).Unix()
				}
				start := time.Now()
				calls := 0
				f.client.Transport = upstreamRoundTripper(func(r *http.Request) (*http.Response, error) {
					require.Less(t, time.Since(start), limit)
					calls++
					if mode == "in-flight" {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					header := make(http.Header)
					if mode == "retry-after" {
						header.Set("Retry-After", "60")
					}
					return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Body: http.NoBody, Header: header}, nil
				})
				f.forward(m, log.With(m))
				require.Equal(t, limit, time.Since(start))
				require.Empty(t, f.retries)
				if mode == "retry-after" || mode == "in-flight" {
					require.Equal(t, 1, calls)
				}
			})
		})
	}
}

func TestUpstreamForwarder_RetryAfter(t *testing.T) {
	for _, mode := range []string{"seconds", "date"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, m := newTestUpstreamForwarder(t)
				start := time.Now()
				calls := 0
				f.client.Transport = upstreamRoundTripper(func(r *http.Request) (*http.Response, error) {
					calls++
					header := make(http.Header)
					status := http.StatusOK
					if calls == 1 {
						status = http.StatusTooManyRequests
						header.Set("Retry-After", "120")
						if mode == "date" {
							header.Set("Retry-After", start.Add(2*time.Minute).UTC().Format(http.TimeFormat))
						}
					}
					return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: http.NoBody, Header: header}, nil
				})
				f.forward(m, log.With(m))
				require.Equal(t, 2, calls)
				require.Equal(t, 2*time.Minute, time.Since(start))
			})
		})
	}
}

func TestUpstreamForwarder_Cancel(t *testing.T) {
	for _, inFlight := range []bool{false, true} {
		t.Run(fmt.Sprint(inFlight), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, m := newTestUpstreamForwarder(t)
				calls := 0
				f.client.Transport = upstreamRoundTripper(func(r *http.Request) (*http.Response, error) {
					calls++
					if inFlight {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Body: http.NoBody, Header: make(http.Header)}, nil
				})
				done := make(chan struct{})
				start := time.Now()
				go func() { f.forward(m, log.With(m)); close(done) }()
				synctest.Wait()
				f.cancel()
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Fatal("forwarding did not stop")
				}
				require.Equal(t, 1, calls)
				require.Equal(t, start, time.Now())
				require.Empty(t, f.retries)
			})
		})
	}
}

func TestUpstreamForwarder_BackoffCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, m := newTestUpstreamForwarder(t)
		var attempts []time.Time
		f.client.Transport = upstreamRoundTripper(func(r *http.Request) (*http.Response, error) {
			attempts = append(attempts, time.Now())
			status := http.StatusServiceUnavailable
			if len(attempts) == 12 {
				status = http.StatusOK
			}
			return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: http.NoBody, Header: make(http.Header)}, nil
		})
		f.forward(m, log.With(m))
		require.Len(t, attempts, 12)
		backoff := upstreamRetryInitialDelay
		for i := 1; i < len(attempts); i++ {
			delay := attempts[i].Sub(attempts[i-1])
			require.GreaterOrEqual(t, delay, backoff/2)
			require.LessOrEqual(t, delay, backoff)
			backoff = min(backoff*2, upstreamRetryMaxDelay)
		}
	})
}

func TestServer_UpstreamRetryStop(t *testing.T) {
	conf := newTestConfig(t, "")
	conf.UpstreamBaseURL = "https://upstream.example"
	conf.UpstreamRetryDuration = time.Minute
	s := newTestServer(t, conf)
	started, canceled := make(chan struct{}), make(chan struct{})
	s.upstream.client.Transport = upstreamRoundTripper(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		close(canceled)
		return nil, r.Context().Err()
	})
	response := request(t, s, http.MethodPut, "/mytopic", "hello", nil)
	require.Equal(t, http.StatusOK, response.Code)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream request did not start")
	}
	s.Stop()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("server shutdown did not cancel upstream request")
	}
}
