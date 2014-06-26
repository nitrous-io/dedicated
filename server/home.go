package server

import (
	"io/ioutil"
	"net/http"
	"path"

	"github.com/nitrous-io/nitrous-common/must"
)

var homeIndexHtml []byte

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if homeIndexHtml == nil {
		homeIndexHtml = must.ByteArray(ioutil.ReadFile(path.Join(s.root, "public", "index.html")))
	}
	s.renderHTML(w, http.StatusOK, homeIndexHtml)
}
