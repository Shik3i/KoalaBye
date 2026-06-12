package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

type Endpoint struct {
	Path           string
	ExpectedStatus int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <base_url>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s http://localhost:8080\n", os.Args[0])
		os.Exit(1)
	}

	baseURL := os.Args[1]
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	endpoints := []Endpoint{
		{"/healthz", http.StatusOK},
		{"/version", http.StatusOK},
		{"/assets/app.css", http.StatusOK},
		{"/setup", http.StatusOK}, // Will follow redirects to /login if already set up
		{"/not-found-404-test", http.StatusNotFound},
	}

	failed := false

	for _, ep := range endpoints {
		target := baseURL + ep.Path
		fmt.Printf("Pinging %s... ", target)

		resp, err := client.Get(target)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			failed = true
			continue
		}

		if resp.StatusCode != ep.ExpectedStatus {
			fmt.Printf("Failed: Expected %d, got %d %s\n", ep.ExpectedStatus, resp.StatusCode, resp.Status)
			failed = true
		} else {
			fmt.Printf("Success (%d)\n", resp.StatusCode)
		}
		resp.Body.Close()
	}

	if failed {
		os.Exit(1)
	}
	fmt.Println("All smoke tests passed.")
}
