package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
)

func (s *Server) Root() string {
	return s.root
}

func (s *Server) Logger() *logger.Logger {
	return s.logger
}

func (s *Server) DB() *db.Database {
	return s.db
}

func (s *Server) RenderHTML(w http.ResponseWriter, status int, html []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(html)))
	w.WriteHeader(status)
	w.Write(html)
	s.logger.Infof("Response: %v", status)
}

func (s *Server) RespondJSON(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	w.Write(data)
	s.logger.Infof("Response: %v %v", status, string(data))
}

func (s *Server) RespondEncodeJSON(w http.ResponseWriter, status int, data map[string]interface{}) {
	j, err := json.Marshal(data)
	if err != nil {
		s.HandleError(w, err)
		return
	}
	s.RespondJSON(w, status, j)
}
