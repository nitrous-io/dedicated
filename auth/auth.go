package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/nitrous-io/nitro/command"
)

func FetchToken(server, username, password string) (string, error) {
	p := url.Values{}
	p.Set("grant_type", "password")
	p.Set("username", username)
	p.Set("password", password)

	url := fmt.Sprintf("%s/api/v0/oauth/token", server)
	response, err := http.PostForm(url, p)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	var message struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		AccessToken      string `json:"access_token"`
	}

	err = json.NewDecoder(response.Body).Decode(&message)
	if err != nil {
		return "", err
	}

	switch response.StatusCode {
	case 401:
		return "", fmt.Errorf(message.ErrorDescription)
	case 200:
		return message.AccessToken, nil
	default:
		return "", fmt.Errorf("failed to authenticate %v", response.Status)
	}
}

func SaveToken(host, username, token string) error {
	c, err := command.LoadConfig()
	if err != nil {
		return err
	}

	authConfig := command.AuthConfig{}
	authConfig.Username = username
	authConfig.Token = token
	c.Auths[host] = authConfig

	return command.SaveConfig(c)
}

func RemoveToken(host string) error {
	c, err := command.LoadConfig()
	if err != nil {
		return err
	}

	delete(c.Auths, host)

	return command.SaveConfig(c)
}
