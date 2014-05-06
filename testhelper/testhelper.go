package testhelper

import (
	"net/http"
	"net/url"
)

func NewRequest(method string, urlStr string, params url.Values) (*http.Request, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	return http.NewRequest(method, u.String(), nil)
}
