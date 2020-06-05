package main

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type OAuthProvider struct {
	ClientID         string
	ClientSecret     string
	TokenEndpoint    string
	FetchAccountData func(token string) (AccountData, error)
}

var oauthProviders = map[string]OAuthProvider{}

func RegisterOAuthProvider(name string, provider OAuthProvider) {
	oauthProviders[name] = provider
}

func OAuthExchangeCodeForToken(provider OAuthProvider, code, state string) (string, error) {
	var (
		buf    = &bytes.Buffer{}
		client = &http.Client{}
	)

	err := json.NewEncoder(buf).Encode(struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Code         string `json:"code"`
		State        string `json:"state"`
	}{provider.ClientID, provider.ClientSecret, code, state})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", provider.TokenEndpoint, buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	type Response struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}
	var tokenResp Response
	err = json.NewDecoder(resp.Body).Decode(&tokenResp)
	if err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}
