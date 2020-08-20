package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func init() {
	RegisterOAuthProvider("github", OAuthProvider{
		ClientID:      env.GithubClientID,
		ClientSecret:  env.GithubClientSecret,
		TokenEndpoint: "https://github.com/login/oauth/access_token",
		FetchAccountData: func(token string) (AccountData, error) {
			resp, err := githubMakeAuthorizedRequest("https://api.github.com/user", token)
			if err != nil {
				return AccountData{}, err
			}
			defer resp.Close()

			type GithubAccountData struct {
				ID        uint      `json:"id"`
				Name      string    `json:"name"`
				Username  string    `json:"login"`
				CreatedAt time.Time `json:"created_at"`
			}

			var user GithubAccountData
			err = json.NewDecoder(resp).Decode(&user)
			if err != nil {
				return AccountData{}, err
			}

			// we convert this to a string so it matches with a more generic UniqueID
			// and because it doesn't break the current network whic hwas using usernames
			stringID := fmt.Sprintf("%v", user.ID)

			accountData := AccountData{
				UniqueID:  stringID,
				Username:  user.Username,
				Name:      user.Name,
				CreatedAt: user.CreatedAt,
			}
			return accountData, nil
		},
	})
}

func githubMakeAuthorizedRequest(url, token string) (io.ReadCloser, error) {
	var client http.Client

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return nil, errors.Errorf("bad Content-Type in Github response: '%v'", contentType)
	} else if resp.StatusCode != 200 {
		return nil, errors.Errorf("bad response from Github API: code %v (url='%v' token='%v')", resp.Status, url, token)
	}
	return resp.Body, nil
}
