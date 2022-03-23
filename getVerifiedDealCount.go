package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type VerifiedDeals struct {
	Count string `json:"count"`
}

/*
 * Returns the amount of verified deals for a Filecoin address
 */
func getVerifiedDealCount(address string) (int, error) {
	// Create HTTP client and request
	client := &http.Client{}
	url := fmt.Sprintf("https://api.filplus.d.interplanetary.one/public/api/getVerifiedDeals/%v?limit=1&page=1", address)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	// Add headers and perform request
	req.Header.Add("x-api-key", env.FilplusApiKey)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}

	// Read the response body
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// Get count from response body
	var verifiedDeals VerifiedDeals
	err = json.Unmarshal(body, &verifiedDeals)
	if err != nil {
		return 0, err
	}

	// Convert count to integer
	count, err := strconv.Atoi(verifiedDeals.Count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
