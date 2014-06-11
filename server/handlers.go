package server

import (
	"io/ioutil"
	"net/http"
	"text/template"

	"github.com/nitrous-io/nitrous-common/must"
)

var homeIndexTmpl = template.Must(template.New("home-index").Parse(string(must.ByteArray(ioutil.ReadFile("templates/home-index.html.tmpl")))))

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, http.StatusOK, homeIndexTmpl, map[string]string{"Message": "Nitrous OK"})
}
