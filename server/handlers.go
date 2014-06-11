package server

import (
	"net/http"
	"text/template"

	"github.com/nitrous-io/nitrous/pkg/must"
)

var homeIndexTmpl = template.Must(template.New("home-index").Parse(string(must.ReadFile("templates/home-index.html.tmpl"))))

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, http.StatusOK, homeIndexTmpl, map[string]string{"Message": "Nitrous OK"})
}
