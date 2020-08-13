package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
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
		log.Println("[error in Github oauth request 1] provider:", provider)
		log.Println("[error in Github oauth request 1] code:", code)
		log.Println("[error in Github oauth request 1] state:", state)
		return "", err
	}

	req, err := http.NewRequest("POST", provider.TokenEndpoint, buf)
	if err != nil {
		log.Println("[error in Github oauth request 2] request JSON:", buf.String())
		log.Println("[error in Github oauth request 2] provider:", provider)
		log.Println("[error in Github oauth request 2] code:", code)
		log.Println("[error in Github oauth request 2] state:", state)
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Println("[error in Github oauth request 3] request JSON:", buf.String())
		log.Println("[error in Github oauth request 3] provider:", provider)
		log.Println("[error in Github oauth request 3] code:", code)
		log.Println("[error in Github oauth request 3] state:", state)
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
		log.Println("[error in Github oauth request 4] request JSON:", buf.String())
		log.Println("[error in Github oauth request 4] response JSON:", rawResp.String())
		log.Println("[error in Github oauth request 4] provider:", provider)
		log.Println("[error in Github oauth request 4] code:", code)
		log.Println("[error in Github oauth request 4] state:", state)
		return "", err
	} else if tokenResp.AccessToken == "" {
		log.Println("NIGHTMARE NIGHTMARE NIGHTMARE, TOKEN IS EMPTY FOR ABSOLUTELY NO REASON!  err =", err, "and rawResponse =", rawResp.String())
		log.Println("NIGHTMARE request JSON =", buf.String())
	}
	return tokenResp.AccessToken, nil
}
