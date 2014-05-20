package test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

func NewRequest(method string, urlStr string, params *url.Values) (*http.Request, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	return http.NewRequest(method, u.String(), nil)
}

func NewFormRequest(method string, urlStr string, params *url.Values) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, strings.NewReader(params.Encode()))
	if err == nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, err
}

func Wait() {
	runtime.Gosched()
	time.Sleep(50 * time.Millisecond)
}

func TempFile(extension string) string {
	return path.Join(os.TempDir(), fmt.Sprintf("freightertest-%v.%v", time.Now().UnixNano(), extension))
}
