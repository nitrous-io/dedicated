package googlerecaptcha

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const reCaptchaVerifyURL = "https://www.google.com/recaptcha/api/siteverify"

type CaptchaVerifier interface {
	Verify(token string) error
}

type GoogleReCaptchaVerifier struct {
	httpClient *http.Client
	url        string
	secret     string
}

func NewVerifier(secret string) *GoogleReCaptchaVerifier {
	return &GoogleReCaptchaVerifier{
		url:        reCaptchaVerifyURL,
		secret:     secret,
		httpClient: &http.Client{}}
}

func (v *GoogleReCaptchaVerifier) Verify(token string) error {
	data := url.Values{}

	data.Set("secret", v.secret)
	data.Set("response", token)

	req, err := http.NewRequest("POST", v.url, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

	res, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	j := map[string]interface{}{}
	json.NewDecoder(res.Body).Decode(&j)

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Verifying reCaptcha error: %+v", j)
	}

	if success, ok := j["success"]; !ok || !success.(bool) {
		return fmt.Errorf("Verifying reCaptcha error codes: %v", j["error-codes"])
	}

	return nil
}
