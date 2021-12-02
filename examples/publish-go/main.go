package main

import (
	"log"
	"net/http"
	"strings"
)

func main() {
	// Without additional headers (priority, tags, title), it's a one liner.
	// Check out https://ntfy.sh/mytopic in your browser after running this.
	http.Post("https://ntfy.sh/mytopic", "text/plain", strings.NewReader("Backup successful 😀"))

	// If you'd like to add title, priority, or tags, it's a little harder.
	// Check out https://ntfy.sh/phil_alerts in your browser.
	req, err := http.NewRequest("POST", "https://ntfy.sh/phil_alerts",
		strings.NewReader("Remote access to phils-laptop detected. Act right away."))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Title", "Unauthorized access detected")
	req.Header.Set("Priority", "urgent")
	req.Header.Set("Tags", "warning,skull")
	if _, err := http.DefaultClient.Do(req); err != nil {
		log.Fatal(err)
	}
}
