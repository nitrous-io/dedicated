package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/freighter/provider"
)

var (
	jsonStatusOK       = []byte(`{"status":"ok"}`)
	jsonStatusAccepted = []byte(`{"status":"accepted"}`)
	jsonStatusCreated  = []byte(`{"status":"created"}`)
	jsonErrorNotFound  = []byte(`{"error":"no such container"}`)
)

type httpFunc func(w http.ResponseWriter, r *http.Request) error

const authHeaderTokenPrefix = "Token "

func (s *Server) withContainerToken(handler httpFunc, permissions ...string) httpFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		var token string

		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, authHeaderTokenPrefix) {
			token = authHeader[len(authHeaderTokenPrefix):]
		} else {
			token = r.FormValue("token")
		}

		if token != "" {
			id := mux.Vars(r)["id"]
			if err := s.provider.VerifyToken(id, token, permissions...); err == nil {
				return handler(w, r)
			}
		}

		w.Header().Set("WWW-Authenticate", `Token realm="freighter", error="invalid_token"`)
		s.respondEncodeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "auth token is invalid"})
		return nil
	}
}

func (s *Server) wrapError(handler httpFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := handler(w, r); err != nil {
			s.handleError(w, err)
		}
	}
}

func (s *Server) handleError(w http.ResponseWriter, err error) {
	logrus.Error(err)
	if err != nil {
		if s.provider.IsNotFoundError(err) {
			s.respond(w, http.StatusNotFound, jsonErrorNotFound)
		} else {
			s.respondWith500(w, err.Error())
		}
	}
}

func (s *Server) cors(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		handler(w, r)
	}
}

func (s *Server) respond(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)

	if uuid := w.Header().Get(NitrousUUIDHeader); uuid != "nolog" {
		logrus.Infof("uuid=%s Response: %v %v", uuid, status, string(data))
	}
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
		logrus.Error(err)
		s.respondEncodeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
	} else {
		s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"result": successStatus, "container": info})
	}
}

// sends nil error to done channel of active cops processes, closing them
func (s *Server) terminateActiveCops(cid string) {
	n := ActiveCops.Len(cid)
	if n == 0 {
		logrus.Debugf("no active cops processes for container %s", cid)
		return
	}
	logrus.Debugf("Terminating %d active cops processes for container %s", n, cid)
	ActiveCops.Terminate(cid)
}
