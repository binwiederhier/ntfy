// Load test program for ntfy staging server.
// Replicates production traffic patterns derived from access.log analysis.
//
// Traffic profile (from ~5M requests over 20 hours):
//   ~71 req/sec average, ~4,300 req/min
//   49.6% poll requests      (GET /TOPIC/json?poll=1&since=ID)
//   21.4% publish POST       (POST /TOPIC with small body)
//    6.2% subscribe stream   (GET /TOPIC/json?since=X, long-lived)
//    4.1% config check       (GET /v1/config)
//    2.3% other topic GET    (GET /TOPIC)
//    2.2% account check      (GET /v1/account)
//    1.9% websocket sub      (GET /TOPIC/ws?since=X)
//    1.5% publish PUT        (PUT /TOPIC with small body)
//    1.5% raw subscribe      (GET /TOPIC/raw?since=X)
//    1.1% json subscribe     (GET /TOPIC/json, no since)
//    0.7% SSE subscribe      (GET /TOPIC/sse?since=X)
//    remaining: static, PATCH, OPTIONS, etc. (omitted)

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"

	"math/big"
	mrand "math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	baseURL    string
	username   string
	password   string
	rps        float64
	scale      float64
	numTopics  int
	subStreams int
	wsStreams  int
	sseStreams int
	rawStreams int
	duration   time.Duration

	totalRequests atomic.Int64
	totalErrors   atomic.Int64
	activeStreams atomic.Int64

	// Error tracking by category
	errMu        sync.Mutex
	recentErrors []string // last N unique error messages
	errorCounts  = make(map[string]int64)
)

func main() {
	flag.StringVar(&baseURL, "url", "https://staging.ntfy.sh", "Base URL of ntfy server")
	flag.StringVar(&username, "user", "", "Username for authentication")
	flag.StringVar(&password, "pass", "", "Password for authentication")
	flag.Float64Var(&rps, "rps", 71, "Target requests per second (default: prod average)")
	flag.Float64Var(&scale, "scale", 1.0, "Scale factor for all load (0.5 = half load, 2.0 = double)")
	flag.IntVar(&numTopics, "topics", 500, "Number of unique topics to use")
	flag.IntVar(&subStreams, "sub-streams", 200, "Number of concurrent JSON streaming subscriptions")
	flag.IntVar(&wsStreams, "ws-streams", 50, "Number of concurrent WebSocket subscriptions")
	flag.IntVar(&sseStreams, "sse-streams", 20, "Number of concurrent SSE subscriptions")
	flag.IntVar(&rawStreams, "raw-streams", 30, "Number of concurrent raw subscriptions")
	flag.DurationVar(&duration, "duration", 10*time.Minute, "Test duration")
	flag.Parse()

	rps *= scale
	subStreams = int(float64(subStreams) * scale)
	wsStreams = int(float64(wsStreams) * scale)
	sseStreams = int(float64(sseStreams) * scale)
	rawStreams = int(float64(rawStreams) * scale)

	topics := generateTopics(numTopics)

	fmt.Printf("ntfy load test\n")
	fmt.Printf("  Target:       %s\n", baseURL)
	fmt.Printf("  RPS:          %.1f\n", rps)
	fmt.Printf("  Scale:        %.1fx\n", scale)
	fmt.Printf("  Topics:       %d\n", numTopics)
	fmt.Printf("  Sub streams:  %d json, %d ws, %d sse, %d raw\n", subStreams, wsStreams, sseStreams, rawStreams)
	fmt.Printf("  Duration:     %s\n", duration)
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Also handle Ctrl+C
	sigCtx, sigCancel := signal.NotifyContext(ctx, os.Interrupt)
	defer sigCancel()
	ctx = sigCtx

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 1000,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Long-lived streaming client (no timeout)
	streamClient := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			MaxIdleConns:        500,
			MaxIdleConnsPerHost: 500,
			IdleConnTimeout:     0,
		},
	}

	var wg sync.WaitGroup

	// Start long-lived streaming subscriptions
	for i := 0; i < subStreams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			streamSubscription(ctx, streamClient, topics, "json")
		}()
	}
	for i := 0; i < wsStreams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wsSubscription(ctx, topics)
		}()
	}
	for i := 0; i < sseStreams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			streamSubscription(ctx, streamClient, topics, "sse")
		}()
	}
	for i := 0; i < rawStreams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			streamSubscription(ctx, streamClient, topics, "raw")
		}()
	}

	// Start request generators based on traffic weights
	// Weights from log analysis (normalized to sum ~100):
	//   poll=49.6, publish_post=21.4, config=4.1, other_get=2.3, account=2.2, publish_put=1.5
	//   Total short-lived weight ≈ 81.1
	type requestType struct {
		name   string
		weight float64
		fn     func(ctx context.Context, client *http.Client, topics []string)
	}

	types := []requestType{
		{"poll", 49.6, doPoll},
		{"publish_post", 21.4, doPublishPost},
		{"config", 4.1, doConfig},
		{"other_get", 2.3, doOtherGet},
		{"account", 2.2, doAccountCheck},
		{"publish_put", 1.5, doPublishPut},
	}

	totalWeight := 0.0
	for _, t := range types {
		totalWeight += t.weight
	}

	for _, t := range types {
		t := t
		typeRPS := rps * (t.weight / totalWeight)
		if typeRPS < 0.1 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			runAtRate(ctx, typeRPS, func() {
				t.fn(ctx, client, topics)
			})
		}()
	}

	// Stats reporter
	wg.Add(1)
	go func() {
		defer wg.Done()
		reportStats(ctx)
	}()

	wg.Wait()
	fmt.Printf("\nDone. Total requests: %d, errors: %d\n", totalRequests.Load(), totalErrors.Load())
}

func trackError(category string, err error) {
	totalErrors.Add(1)
	key := fmt.Sprintf("%s: %s", category, truncateErr(err))
	errMu.Lock()
	errorCounts[key]++
	errMu.Unlock()
}

func trackErrorMsg(category string, msg string) {
	totalErrors.Add(1)
	key := fmt.Sprintf("%s: %s", category, msg)
	errMu.Lock()
	errorCounts[key]++
	errMu.Unlock()
}

func truncateErr(err error) string {
	s := err.Error()
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}

func setAuth(req *http.Request) {
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}
}

func generateTopics(n int) []string {
	topics := make([]string, n)
	for i := 0; i < n; i++ {
		b := make([]byte, 8)
		rand.Read(b)
		topics[i] = "loadtest-" + hex.EncodeToString(b)
	}
	return topics
}

func pickTopic(topics []string) string {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(topics))))
	return topics[n.Int64()]
}

func randomSince() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func randomMessage() string {
	messages := []string{
		"Test notification",
		"Server backup completed successfully",
		"Deployment finished",
		"Alert: disk usage above 80%",
		"Build #1234 passed",
		"New order received",
		"Temperature sensor reading: 72F",
		"Cron job completed",
	}
	return messages[mrand.Intn(len(messages))]
}

// runAtRate executes fn at approximately the given rate per second
func runAtRate(ctx context.Context, rate float64, fn func()) {
	interval := time.Duration(float64(time.Second) / rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			go fn()
		}
	}
}

// --- Short-lived request types ---

func doPoll(ctx context.Context, client *http.Client, topics []string) {
	topic := pickTopic(topics)
	url := fmt.Sprintf("%s/%s/json?poll=1&since=%s", baseURL, topic, randomSince())
	doGet(ctx, client, url)
}

func doPublishPost(ctx context.Context, client *http.Client, topics []string) {
	topic := pickTopic(topics)
	url := fmt.Sprintf("%s/%s", baseURL, topic)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(randomMessage()))
	if err != nil {
		trackError("publish_post_req", err)
		return
	}
	setAuth(req)
	// Some messages have titles/priorities like real traffic
	if mrand.Float32() < 0.3 {
		req.Header.Set("X-Title", "Load Test")
	}
	if mrand.Float32() < 0.1 {
		req.Header.Set("X-Priority", fmt.Sprintf("%d", mrand.Intn(5)+1))
	}
	resp, err := client.Do(req)
	totalRequests.Add(1)
	if err != nil {
		trackError("publish_post", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		trackErrorMsg("publish_post_http", fmt.Sprintf("status %d", resp.StatusCode))
	}
}

func doPublishPut(ctx context.Context, client *http.Client, topics []string) {
	topic := pickTopic(topics)
	url := fmt.Sprintf("%s/%s", baseURL, topic)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(randomMessage()))
	if err != nil {
		trackError("publish_put_req", err)
		return
	}
	setAuth(req)
	resp, err := client.Do(req)
	totalRequests.Add(1)
	if err != nil {
		trackError("publish_put", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		trackErrorMsg("publish_put_http", fmt.Sprintf("status %d", resp.StatusCode))
	}
}

func doConfig(ctx context.Context, client *http.Client, topics []string) {
	url := fmt.Sprintf("%s/v1/config", baseURL)
	doGet(ctx, client, url)
}

func doAccountCheck(ctx context.Context, client *http.Client, topics []string) {
	url := fmt.Sprintf("%s/v1/account", baseURL)
	doGet(ctx, client, url)
}

func doOtherGet(ctx context.Context, client *http.Client, topics []string) {
	topic := pickTopic(topics)
	url := fmt.Sprintf("%s/%s", baseURL, topic)
	doGet(ctx, client, url)
}

func doGet(ctx context.Context, client *http.Client, url string) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		trackError("get_req", err)
		return
	}
	setAuth(req)
	resp, err := client.Do(req)
	totalRequests.Add(1)
	if err != nil {
		trackError("get", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		trackErrorMsg("get_http", fmt.Sprintf("status %d for %s", resp.StatusCode, url))
	}
}

// --- Long-lived streaming subscriptions ---

func streamSubscription(ctx context.Context, client *http.Client, topics []string, format string) {
	for {
		if ctx.Err() != nil {
			return
		}
		topic := pickTopic(topics)
		url := fmt.Sprintf("%s/%s/%s?since=all", baseURL, topic, format)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		setAuth(req)
		activeStreams.Add(1)
		resp, err := client.Do(req)
		if err != nil {
			activeStreams.Add(-1)
			if ctx.Err() == nil {
				trackError("stream_"+format+"_connect", err)
			}
			time.Sleep(time.Second)
			continue
		}
		if resp.StatusCode >= 400 {
			trackErrorMsg("stream_"+format+"_http", fmt.Sprintf("status %d", resp.StatusCode))
			resp.Body.Close()
			activeStreams.Add(-1)
			time.Sleep(time.Second)
			continue
		}
		// Read from stream until context cancelled or connection drops
		buf := make([]byte, 4096)
		for {
			_, err := resp.Body.Read(buf)
			if err != nil {
				if ctx.Err() == nil {
					trackError("stream_"+format+"_read", err)
				}
				break
			}
		}
		resp.Body.Close()
		activeStreams.Add(-1)
		// Reconnect with small delay (like real clients do)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(mrand.Intn(3000)) * time.Millisecond):
		}
	}
}

func wsSubscription(ctx context.Context, topics []string) {
	wsURL := strings.Replace(baseURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	for {
		if ctx.Err() != nil {
			return
		}
		topic := pickTopic(topics)
		url := fmt.Sprintf("%s/%s/ws?since=all", wsURL, topic)

		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}
		var wsHeader http.Header
		if username != "" && password != "" {
			wsHeader = http.Header{}
			req, _ := http.NewRequest("GET", url, nil)
			req.SetBasicAuth(username, password)
			wsHeader.Set("Authorization", req.Header.Get("Authorization"))
		}
		activeStreams.Add(1)
		conn, _, err := dialer.DialContext(ctx, url, wsHeader)
		if err != nil {
			activeStreams.Add(-1)
			if ctx.Err() == nil {
				trackError("ws_connect", err)
			}
			time.Sleep(time.Second)
			continue
		}

		// Read messages until context cancelled or error
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
				_, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
			}
		}()

		select {
		case <-ctx.Done():
			conn.Close()
			activeStreams.Add(-1)
			return
		case <-done:
			conn.Close()
			activeStreams.Add(-1)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(mrand.Intn(3000)) * time.Millisecond):
		}
	}
}

func reportStats(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastRequests, lastErrors int64
	lastTime := time.Now()
	reportCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			currentRequests := totalRequests.Load()
			currentErrors := totalErrors.Load()
			elapsed := now.Sub(lastTime).Seconds()
			currentRPS := float64(currentRequests-lastRequests) / elapsed
			errorRate := float64(currentErrors-lastErrors) / elapsed

			fmt.Printf("[%s] rps=%.1f err/s=%.1f total=%d errors=%d streams=%d\n",
				now.Format("15:04:05"),
				currentRPS,
				errorRate,
				currentRequests,
				currentErrors,
				activeStreams.Load(),
			)

			// Print error breakdown every 30 seconds
			reportCount++
			if reportCount%6 == 0 && currentErrors > 0 {
				errMu.Lock()
				fmt.Printf("  Error breakdown:\n")
				for k, v := range errorCounts {
					fmt.Printf("    %s: %d\n", k, v)
				}
				errMu.Unlock()
			}

			lastRequests = currentRequests
			lastErrors = currentErrors
			lastTime = now
		}
	}
}
