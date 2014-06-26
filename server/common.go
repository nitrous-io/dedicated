package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
)

var (
	jsonStatusOK       = []byte(`{"status":"ok"}`)
	jsonStatusAccepted = []byte(`{"status":"accepted"}`)
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

	s.renderHTML(w, status, buf.Bytes())
}

func (s *Server) renderHTML(w http.ResponseWriter, status int, html []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(html)
	s.logger.Infof("Response: %v", status)
}

func (s *Server) respond(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)
	s.logger.Infof("Response: %v %v", status, string(data))
}

func (s *Server) respondEncodeJson(w http.ResponseWriter, status int, data map[string]interface{}) {
	j, err := json.Marshal(data)
	if err != nil {
		s.respondWith500(w, "json marshaling failed")
		return
	}
	s.respond(w, status, j)
}

func (s *Server) respondWith500(w http.ResponseWriter, errMsg string) {
	e := map[string]string{"error": errMsg}
	j, err := json.Marshal(e)
	if err != nil {
		j = []byte("json marshaling failed")
	}
	s.respond(w, http.StatusInternalServerError, []byte(j))
}
