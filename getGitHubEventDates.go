package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type GithubEvent struct {
	CreatedAt string `json:"created_at"`
}

/*
 * Get all "created_at" dates for GitHub events belonging to the provided GitHub account
 */
func getGitHubEventDates(account string) ([]string, error) {
	dates := []string{}
	url := fmt.Sprintf("https://api.github.com/users/%v/events?per_page=100", account)
	for url != "" {
		next, err := addDatesFromGitHubEventPage(url, dates)
		if err != nil {
			return nil, err
		}
		url = next
	}
	return dates, nil
}

/*
 * Adds all the "created_at" dates found in the GitHub event page to the provided
 * "dates" slice and returns the url for the next GitHub event page, if it exists
 */
func addDatesFromGitHubEventPage(url string, dates []string) (string, error) {
	// Create HTTP client and request
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Add headers and perform request
	req.Header.Add("accept", "application/vnd.github.v3+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	// Read the response body
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Get events from response body
	var githubEvents []GithubEvent
	err = json.Unmarshal(body, &githubEvents)
	if err != nil {
		return "", err
	}

	// Append "created_at" dates
	for _, event := range githubEvents {
		dates = append(dates, event.CreatedAt)
	}

	// Retrieve the next page URL
	link := resp.Header.Get("link")
	next := getLinkHeaderURI(link, "next")
	return next, nil
}
