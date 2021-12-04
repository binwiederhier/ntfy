package main

import (
	"bufio"
	"log"
	"net/http"
)

func main() {
	resp, err := http.Get("https://ntfy.sh/phil_alerts/json")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		println(scanner.Text())
	}
}
