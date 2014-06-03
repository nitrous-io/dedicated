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

func (self *Server) handleError(w http.ResponseWriter, err error) {
	self.logger.Error(err)
	if err != nil {
		if self.provider.IsNotFoundError(err) {
			self.respond(w, http.StatusNotFound, jsonErrorNotFound)
		} else {
			self.respondWith500(w, err.Error())
		}
	}
}

func (self *Server) respond(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)
	self.logger.Infof("Response: %v %v", status, string(data))
}

func (self *Server) respondEncodeJson(w http.ResponseWriter, status int, data map[string]interface{}) {
	j, err := json.Marshal(data)
	if err != nil {
		self.respondWith500(w, "json marshaling failed")
		return
	}
	self.respond(w, status, j)
}

func (self *Server) respondWith500(w http.ResponseWriter, errMsg string) {
	e := map[string]string{"error": errMsg}
	j, err := json.Marshal(e)
	if err != nil {
		j = []byte("json marshaling failed")
	}
	self.respond(w, http.StatusInternalServerError, []byte(j))
}

func (self *Server) performContainerControl(w http.ResponseWriter, r *http.Request, controlFunc func(string) (*provider.ContainerInfo, error), successStatus string) {
	id := mux.Vars(r)["id"]
	callbackUrl := r.FormValue("callback_url")
	state := r.FormValue("state")

	_, err := self.provider.GetInfo(id)
	if err != nil {
		self.handleError(w, err)
		return
	}

	go func() {
		info, err := controlFunc(id)

		self.invokeCallback(callbackUrl, state, map[string]interface{}{
			"result":    successStatus,
			"container": info,
		}, err)
	}()

	self.respond(w, http.StatusAccepted, jsonStatusAccepted)
}

func (self *Server) invokeCallback(callbackUrl string, state string, data map[string]interface{}, err error) {
	if callbackUrl == "" {
		return
	}
	var res map[string]interface{}

	if err != nil {
		self.logger.Error(err)
		res = map[string]interface{}{"error": err.Error()}
	} else {
		res = data
	}
	if state != "" {
		res["state"] = state
	}
	j, err := json.Marshal(res)
	if err != nil {
		self.logger.Error(err)
		return
	}
	sj := string(j)
	self.logger.Infof("Callback: %v %v", callbackUrl, sj)
	self.callback.Invoke(callbackUrl, sj)
}
