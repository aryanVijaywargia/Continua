package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	url := os.Getenv("HEALTH_URL")
	if url == "" {
		url = "http://localhost:8243/health"
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}
