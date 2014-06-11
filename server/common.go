package server

import (
	"bytes"
	"fmt"
	"net/http"
	"text/template"
)

func (s *Server) renderTemplate(w http.ResponseWriter, status int, tmpl *template.Template, data interface{}) {
	buf := &bytes.Buffer{}
	err := tmpl.Execute(buf, data)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Error: %v", err)))
		s.logger.Error(err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
	s.logger.Infof("Response: %v", status)
}
