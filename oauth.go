package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type OAuthProvider struct {
	ClientID         string
	ClientSecret     string
	TokenEndpoint    string
	FetchAccountData func(token string) (AccountData, error)
}

type GithubOAuthRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Code         string `json:"code"`
	State        string `json:"state"`
}

type GithubOAuthResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

var oauthProviders = map[string]OAuthProvider{}

func RegisterOAuthProvider(name string, provider OAuthProvider) {
	oauthProviders[name] = provider
}

func OAuthExchangeCodeForToken(provider OAuthProvider, code, state string) (string, error) {
	// Create the request body
	reqBody, err := json.Marshal(GithubOAuthRequest{provider.ClientID, provider.ClientSecret, code, state})
	if err != nil {
		return "", err
	}

	// Create HTTP client and request
	client := &http.Client{}
	req, err := http.NewRequest("POST", provider.TokenEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}

	// Set headers and perform request
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	// Read the response body
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse the response body
	var oAuthResp GithubOAuthResponse
	err = json.Unmarshal(respBody, &oAuthResp)
	if err != nil {
		return "", err
	}
	if oAuthResp.AccessToken == "" {
		return "", errors.New("GitHub returned empty OAuth access token")
	}

	return oAuthResp.AccessToken, nil
}
