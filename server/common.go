package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
)

var (
	jsonStatusOK       = []byte(`{"status":"ok"}`)
	jsonStatusAccepted = []byte(`{"status":"accepted"}`)
	jsonErrorNotFound  = []byte(`{"error":"no such container"}`)
)

func (s *Server) handleError(w http.ResponseWriter, err error) {
	s.logger.Error(err)
	if err != nil {
		if s.provider.IsNotFoundError(err) {
			s.respond(w, http.StatusNotFound, jsonErrorNotFound)
		} else {
			s.respondWith500(w, err.Error())
		}
	}
}

func (s *Server) respond(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)
	s.logger.Infof("Response: %v %v", status, string(data))
}

func (s *Server) respondEncodeJSON(w http.ResponseWriter, status int, data map[string]interface{}) {
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

func (s *Server) performContainerControl(w http.ResponseWriter, r *http.Request, controlFunc func(string) (*provider.ContainerInfo, error), successStatus string) {
	id := mux.Vars(r)["id"]

	_, err := s.provider.GetInfo(id)
	if err != nil {
		s.handleError(w, err)
		return
	}

	info, err := controlFunc(id)

	if err != nil {
		s.logger.Error(err)
		s.respondEncodeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
	} else {
		s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"result": successStatus, "container": info})
	}
}
