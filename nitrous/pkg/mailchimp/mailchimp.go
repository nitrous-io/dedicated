package mailchimp

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
)

var (
	ErrMailChimpAPIUrlNotSet = errors.New("MAILCHIMP_API_URL not set")
	ErrMailChimpAPIKeyNotSet = errors.New("MAILCHIMP_API_KEY not set")

	ErrNoListID = errors.New("listID must be provided")

	ErrStatusNotOK = errors.New("HTTP status not 200 OK")
)

type SubscribeOptions struct {
	Email       string                 `json:"email_address"`
	Status      string                 `json:"status"`
	MergeFields map[string]interface{} `json:"merge_fields,omitempty"`
}

type MailChimpProvider interface {
	Subscribe(listID string, opts *SubscribeOptions) error
	Unsubscribe(listID, email string) error
}

type MailChimp struct {
}

func (m *MailChimp) Subscribe(listID string, opts *SubscribeOptions) error {
	if listID == "" {
		return ErrNoListID
	}

	if opts.Status == "" {
		opts.Status = "subscribed"
	}
	path := fmt.Sprintf("lists/%s/members", listID)
	return m.doRequest(path, "POST", opts)
}

func (m *MailChimp) Unsubscribe(listID, email string) error {
	if listID == "" {
		return ErrNoListID
	}

	email = strings.ToLower(email)
	md5s := md5.Sum([]byte(email))
	md5sum := hex.EncodeToString(md5s[:])
	path := fmt.Sprintf("lists/%s/members/%s", listID, md5sum)
	return m.doRequest(path, "DELETE", nil)
}

func (m *MailChimp) doRequest(path, method string, data interface{}) error {
	if envvars.MailChimpAPIUrl == "" {
		return ErrMailChimpAPIUrlNotSet
	}

	if envvars.MailChimpAPIKey == "" {
		return ErrMailChimpAPIKeyNotSet
	}

	var bodyReader io.Reader
	if data != nil {
		json, err := json.Marshal(data)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewBuffer(json)
	}

	url := fmt.Sprintf("%s/3.0/%s", envvars.MailChimpAPIUrl, path)
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return err
	}

	req.SetBasicAuth("ignored", envvars.MailChimpAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode == http.StatusOK {
		return nil
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return fmt.Errorf("Error performing %q, err: %q", path, body)
}
