package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
)

func (self *Server) home(w http.ResponseWriter, r *http.Request) {
	self.respond(w, http.StatusOK, jsonStatusOK)
}

func (self *Server) listContainers(w http.ResponseWriter, r *http.Request) {
	infoList, err := self.provider.List()
	if err != nil {
		self.handleError(w, err)
		return
	}

	self.respondEncodeJson(w, http.StatusOK, map[string]interface{}{"containers": infoList})
}

func (self *Server) createContainer(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	image := r.FormValue("image")
	callbackUrl := r.FormValue("callback_url")
	state := r.FormValue("state")
	publicKeys := r.PostForm["public_key[]"]

	if publicKeys == nil {
		publicKeys = []string{}
	}

	if name == "" || image == "" || !provider.IsValidName(name) {
		self.respond(w, 422, []byte(`{"error":"invalid params"}`))
		return
	}

	go func() {
		info, err := self.provider.Create(name, image, publicKeys)
		self.invokeCallback(callbackUrl, state, map[string]interface{}{
			"result":    "created",
			"container": info,
		}, err)
	}()

	self.respond(w, http.StatusAccepted, jsonStatusAccepted)
}

func (self *Server) getContainerInfo(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	info, err := self.provider.GetInfo(id)
	if err != nil {
		self.handleError(w, err)
		return
	}

	self.respondEncodeJson(w, http.StatusOK, map[string]interface{}{"container": info})
}

func (self *Server) terminateContainer(w http.ResponseWriter, r *http.Request) {
	self.performContainerControl(w, r, self.provider.Terminate, "terminated")
}

func (self *Server) startContainer(w http.ResponseWriter, r *http.Request) {
	self.performContainerControl(w, r, self.provider.Start, "started")
}

func (self *Server) stopContainer(w http.ResponseWriter, r *http.Request) {
	self.performContainerControl(w, r, self.provider.Stop, "stopped")
}

func (self *Server) restartContainer(w http.ResponseWriter, r *http.Request) {
	self.performContainerControl(w, r, self.provider.Restart, "restarted")
}

func (self *Server) listPublicKeys(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	allPubKeys, err := self.provider.ListPublicKeys(id)
	if err != nil {
		self.handleError(w, err)
		return
	}

	self.respondEncodeJson(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}

func (self *Server) addPublicKeys(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	err := r.ParseForm()
	if err != nil {
		self.handleError(w, err)
		return
	}

	publicKeys := r.PostForm["public_key[]"]

	if publicKeys == nil {
		self.respond(w, 422, []byte(`{"error":"invalid params"}`))
		return
	}

	err = self.provider.AddPublicKeys(id, publicKeys)
	if err != nil {
		self.handleError(w, err)
		return
	}

	allPubKeys, err := self.provider.ListPublicKeys(id)
	if err != nil {
		self.handleError(w, err)
		return
	}

	self.respondEncodeJson(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}

func (self *Server) removePublicKey(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	keyFingerprint := mux.Vars(r)["key_fingerprint"]

	err := self.provider.RemovePublicKey(id, keyFingerprint)
	if err != nil {
		self.handleError(w, err)
		return
	}

	allPubKeys, err := self.provider.ListPublicKeys(id)
	if err != nil {
		self.handleError(w, err)
		return
	}

	self.respondEncodeJson(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}
