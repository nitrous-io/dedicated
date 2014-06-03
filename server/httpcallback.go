package server

import (
	"bytes"
	"net/http"
)

type HTTPCallback struct{}

func (h *HTTPCallback) Invoke(to string, payload string) {
	buf := bytes.NewBufferString(payload)
	res, _ := http.Post(to, "application/json; charset=utf-8", buf)
	if res != nil && res.Body != nil {
		defer res.Body.Close()
	}
}
