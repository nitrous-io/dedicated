package home

import (
	"io/ioutil"
	"net/http"
	"path"

	"github.com/nitrous-io/nitrous-common/must"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
)

var homeIndexHtml []byte

func Index(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	if homeIndexHtml == nil {
		homeIndexHtml = must.ByteArray(ioutil.ReadFile(path.Join(c.Root(), "public", "index.html")))
	}
	c.RenderHTML(w, http.StatusOK, homeIndexHtml)
	return nil
}
