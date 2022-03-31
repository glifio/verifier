package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/glifio/go-logger"
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
		logger.Errorf("[Github oauth request 1] provider: %v, code: %v, state: %v", provider, code, state)
		return "", err
	}

	req, err := http.NewRequest("POST", provider.TokenEndpoint, buf)
	if err != nil {
		logger.Errorf("[Github oauth request 2] request JSON: %v, provider: %v, code: %v, state: %v", buf.String(), provider, code, state)
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("[Github oauth request 3] request JSON: %v, provider: %v, code: %v, state: %v", buf.String(), provider, code, state)
		return "", err
	}
	defer resp.Body.Close()

	type Response struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}
	var tokenResp Response
	rawResp := &bytes.Buffer{}
	err = json.NewDecoder(io.TeeReader(resp.Body, rawResp)).Decode(&tokenResp)
	if err != nil {
		logger.Errorf("[Github oauth request 4] request JSON: %v, response JSON: %v, provider: %v, code: %v, state: %v", buf.String(), rawResp.String(), provider, code, state)
		return "", err
	} else if tokenResp.AccessToken == "" {
		logger.Errorf("[Github oauth token empty] request JSON: %v, response JSON: %v, provider: %v, code: %v, state: %v", buf.String(), rawResp.String(), provider, code, state)
	}
	return tokenResp.AccessToken, nil
}
