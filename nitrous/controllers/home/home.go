package home

import (
	"io/ioutil"
	"net/http"
	"path"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/nitrous-common/must"
)

var homeIndexHtml []byte

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if homeIndexHtml == nil {
		homeIndexHtml = must.ByteArray(ioutil.ReadFile(path.Join(env.Root, "public", "index.html")))
	}
	renderHTML(w, http.StatusOK, homeIndexHtml)
	return nil
}

func renderHTML(w http.ResponseWriter, status int, html []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(html)))
	w.WriteHeader(status)
	w.Write(html)
}
