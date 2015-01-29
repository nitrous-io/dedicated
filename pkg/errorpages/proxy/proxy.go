package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

type nopCloser struct {
	io.Reader
}

func (c nopCloser) Close() error {
	return nil
}

func ErrorHandler(res *http.Response, err error) {
	res.StatusCode = http.StatusServiceUnavailable

	h := http.Header{}
	h.Set("Content-Type", "text/html")
	h.Add("Vary", "Accept-Encoding")
	res.Header = h

	if strings.Contains(res.Request.Header.Get("Accept-Encoding"), "gzip") {
		res.Header.Set("Content-Encoding", "gzip")
		res.Body = nopCloser{bytes.NewBuffer(GzippedErrorPage)}
	} else {
		res.Body = nopCloser{bytes.NewBuffer(ErrorPage)}
	}
}
