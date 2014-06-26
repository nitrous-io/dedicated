package home

import (
	"io/ioutil"
	"net/http"
	"path"

	"github.com/nitrous-io/nitrous-common/must"
	"github.com/nitrous-io/nitrous/controllers"
)

var homeIndexHtml []byte

func Index(c controllers.Helper, w http.ResponseWriter, r *http.Request) {
	if homeIndexHtml == nil {
		homeIndexHtml = must.ByteArray(ioutil.ReadFile(path.Join(c.Root(), "public", "index.html")))
	}
	c.RenderHTML(w, http.StatusOK, homeIndexHtml)
}
