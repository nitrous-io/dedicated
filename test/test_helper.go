package test

import (
	"fmt"
	"math/rand"
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
	return path.Join(os.TempDir(), fmt.Sprintf("%v.%v", RandomName("freightertest"), extension))
}

func TempDir() (string, error) {
	dir := path.Join(os.TempDir(), RandomName("freightertest"))
	err := os.Mkdir(dir, 0700)
	if err != nil {
		return "", err
	}
	return dir, nil
}

func RandomName(name string) string {
	return fmt.Sprintf("%v-%v-%v", name, time.Now().UnixNano(), rand.Intn(1000))
}
