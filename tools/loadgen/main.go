package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	baseURL := "https://staging.ntfy.sh"
	if len(os.Args) > 1 {
		baseURL = os.Args[1]
	}
	for i := 0; i < 2000; i++ {
		go subscribe(i, baseURL)
	}
	time.Sleep(5 * time.Second)
	for i := 0; i < 2000; i++ {
		go func(worker int) {
			for {
				poll(worker, baseURL)
			}
		}(i)
	}
	time.Sleep(time.Hour)
}

func subscribe(worker int, baseURL string) {
	fmt.Printf("[subscribe] worker=%d STARTING\n", worker)
	start := time.Now()
	topic, ip := fmt.Sprintf("subtopic%d", worker), fmt.Sprintf("1.2.%d.%d", (worker/255)%255, worker%255)
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/%s/json", baseURL, topic), nil)
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[subscribe] worker=%d time=%d error=%s\n", worker, time.Since(start).Milliseconds(), err.Error())
		return
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		// Do nothing
	}
	fmt.Printf("[subscribe] worker=%d status=%d time=%d EXITED\n", worker, resp.StatusCode, time.Since(start).Milliseconds())
}

func poll(worker int, baseURL string) {
	fmt.Printf("[poll] worker=%d STARTING\n", worker)
	topic, ip := fmt.Sprintf("polltopic%d", worker), fmt.Sprintf("1.2.%d.%d", (worker/255)%255, worker%255)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	//req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://staging.ntfy.sh/%s/json?poll=1&since=all", topic), nil)
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/%s/json?poll=1&since=all", baseURL, topic), nil)
	req.Header.Set("X-Forwarded-For", ip)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[poll] worker=%d time=%d status=- error=%s\n", worker, time.Since(start).Milliseconds(), err.Error())
		cancel()
		return
	}
	defer resp.Body.Close()
	fmt.Printf("[poll] worker=%d time=%d status=%s\n", worker, time.Since(start).Milliseconds(), resp.Status)
}
