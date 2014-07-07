// httpclient is a wrapper for net/http.Client with convenience methods for
// authenticated requests
package httpclient

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	*http.Client
	AuthToken string
}

var DefaultClient = &Client{http.DefaultClient, ""}

// New returns a client that uses net/http's DefaultClient with the token given by authToken.  If authToken is the empty string, authed*() calls will fail with a "must be logged in" error message
func New(authToken string) *Client {
	return &Client{http.DefaultClient, authToken}
}

// ResponseJSON takes a response expected to be JSON and unmarshals it
func ResponseJSON(resp *http.Response) (map[string]interface{}, error) {
	j := map[string]interface{}{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &j)
	return j, err
}

func (c *Client) AuthedGet(urlStr string, params *url.Values) (*http.Response, error) {
	return c.DoAuthedRequest("GET", urlStr, params)
}

func (c *Client) AuthedDelete(urlStr string, params *url.Values) (*http.Response, error) {
	return c.DoAuthedRequest("DELETE", urlStr, params)
}

func (c *Client) AuthedPostForm(urlStr string, data *url.Values) (*http.Response, error) {
	req, err := c.prepareForm("POST", urlStr, data)
	if err != nil {
		return nil, err
	}
	return c.authedDo(req)
}

func (c *Client) AuthedPutForm(urlStr string, data *url.Values) (*http.Response, error) {
	req, err := c.prepareForm("PUT", urlStr, data)
	if err != nil {
		return nil, err
	}
	return c.authedDo(req)
}

func (c *Client) Delete(urlStr string, params *url.Values) (*http.Response, error) {
	return c.DoRequest("DELETE", urlStr, params)
}

func (c *Client) Get(urlStr string, params *url.Values) (*http.Response, error) {
	return c.DoRequest("GET", urlStr, params)
}

func (c *Client) PutForm(urlStr string, data *url.Values) (*http.Response, error) {
	req, err := c.prepareForm("PUT", urlStr, data)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *Client) DoRequest(method string, urlStr string, params *url.Values) (*http.Response, error) {
	req, err := c.prepareRequest(method, urlStr, params)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *Client) DoAuthedRequest(method string, urlStr string, params *url.Values) (*http.Response, error) {
	req, err := c.prepareRequest(method, urlStr, params)
	if err != nil {
		return nil, err
	}
	return c.authedDo(req)
}

func (c *Client) DoForm(method string, urlStr string, data *url.Values) (*http.Response, error) {
	req, err := c.prepareForm(method, urlStr, data)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *Client) DoAuthedForm(method string, urlStr string, data *url.Values) (*http.Response, error) {
	req, err := c.prepareForm(method, urlStr, data)
	if err != nil {
		return nil, err
	}
	return c.authedDo(req)
}

func (c *Client) prepareRequest(method string, urlStr string, params *url.Values) (*http.Request, error) {
	if method != "GET" && method != "DELETE" {
		return nil, fmt.Errorf("method argument must be GET or DELETE")
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// prepareForm is modeled after net/http Client.Post
func (c *Client) prepareForm(method string, urlStr string, data *url.Values) (*http.Request, error) {
	if method != "POST" && method != "PUT" {
		return nil, fmt.Errorf("method argument must be POST or PUT")
	}
	var body *strings.Reader
	if data == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(data.Encode())
	}
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}

func (c *Client) authedDo(req *http.Request) (*http.Response, error) {
	if c.AuthToken == "" {
		return nil, fmt.Errorf("You must be logged in to do that")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AuthToken))
	return c.Do(req)
}
