package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
)

var (
	jsonStatusOK       = []byte(`{"status":"ok"}`)
	jsonStatusAccepted = []byte(`{"status":"accepted"}`)
	jsonErrorNotFound  = []byte(`{"error":"not found"}`)
)

type RestAPI struct {
	Provider provider.Provider
	Callback Callback
}

func (a *RestAPI) Home(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, jsonStatusOK)
}

func (a *RestAPI) ListContainers(w http.ResponseWriter, r *http.Request) {
	infoList, err := a.Provider.List()
	if err != nil {
		respondWith500(w, "failed to fetch container list")
		return
	}

	respondEncodeJson(w, http.StatusOK, map[string]interface{}{"containers": infoList})
}

func (a *RestAPI) CreateContainer(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	image := r.FormValue("image")
	callbackUrl := r.FormValue("callback_url")
	state := r.FormValue("state")
	publicKeys := r.PostForm["public_key[]"]

	if publicKeys == nil {
		publicKeys = []string{}
	}

	if name == "" || image == "" {
		respond(w, 422, []byte(`{"error":"invalid params"}`))
		return
	}

	go func() {
		info, err := a.Provider.Create(name, image, publicKeys)
		if callbackUrl == "" {
			return
		}
		if err != nil {
			res := map[string]string{"error": err.Error()}
			if state != "" {
				res["state"] = state
			}
			j, err := json.Marshal(res)
			if err == nil {
				a.Callback.Invoke(callbackUrl, string(j))
			}
			return
		}
		res := map[string]interface{}{"result": "created", "container": info}
		if state != "" {
			res["state"] = state
		}
		j, err := json.Marshal(res)
		if err == nil {
			a.Callback.Invoke(callbackUrl, string(j))
		}
	}()

	respond(w, http.StatusAccepted, jsonStatusAccepted)
}

func (a *RestAPI) GetContainerInfo(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	info, err := a.Provider.GetInfo(id)

	if err != nil {
		respond(w, http.StatusNotFound, jsonErrorNotFound)
		return
	}

	respondEncodeJson(w, http.StatusOK, map[string]interface{}{"container": info})
}

func (a *RestAPI) TerminateContainer(w http.ResponseWriter, r *http.Request) {
	a.performContainerControl(w, r, a.Provider.Terminate, "terminated")
}

func (a *RestAPI) StartContainer(w http.ResponseWriter, r *http.Request) {
	a.performContainerControl(w, r, a.Provider.Start, "started")
}

func (a *RestAPI) StopContainer(w http.ResponseWriter, r *http.Request) {
	a.performContainerControl(w, r, a.Provider.Stop, "stopped")
}

func (a *RestAPI) RestartContainer(w http.ResponseWriter, r *http.Request) {
	a.performContainerControl(w, r, a.Provider.Restart, "restarted")
}

func (a *RestAPI) ListPublicKeys(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	allPubKeys, err := a.Provider.ListPublicKeys(id)
	if err != nil {
		respondEncodeJson(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	respondEncodeJson(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}

func (a *RestAPI) AddPublicKeys(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	err := r.ParseForm()
	if err != nil {
		respondEncodeJson(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	publicKeys := r.PostForm["public_key[]"]

	if publicKeys == nil {
		respond(w, 422, []byte(`{"error":"invalid params"}`))
		return
	}

	err = a.Provider.AddPublicKeys(id, publicKeys)
	if err != nil {
		respondEncodeJson(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	allPubKeys, err := a.Provider.ListPublicKeys(id)
	if err != nil {
		respondEncodeJson(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	respondEncodeJson(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}

func (a *RestAPI) RemovePublicKey(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	keyFingerprint := mux.Vars(r)["key_fingerprint"]

	err := a.Provider.RemovePublicKey(id, keyFingerprint)
	if err != nil {
		respondEncodeJson(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	allPubKeys, err := a.Provider.ListPublicKeys(id)
	if err != nil {
		respondEncodeJson(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	respondEncodeJson(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}

func (a *RestAPI) performContainerControl(w http.ResponseWriter, r *http.Request, controlFunc func(string) error, successStatus string) {
	id := mux.Vars(r)["id"]
	callbackUrl := r.FormValue("callback_url")
	state := r.FormValue("state")

	_, err := a.Provider.GetInfo(id)

	if err != nil {
		respond(w, http.StatusNotFound, jsonErrorNotFound)
		return
	}

	go func() {
		err := controlFunc(id)
		if callbackUrl == "" {
			return
		}
		if err != nil {
			res := map[string]string{"error": err.Error()}
			if state != "" {
				res["state"] = state
			}
			j, err := json.Marshal(res)
			if err == nil {
				a.Callback.Invoke(callbackUrl, string(j))
			}
			return
		}
		res := map[string]string{"result": successStatus, "container_id": id}
		if state != "" {
			res["state"] = state
		}
		j, err := json.Marshal(res)
		if err == nil {
			a.Callback.Invoke(callbackUrl, string(j))
		}
	}()

	respond(w, http.StatusAccepted, jsonStatusAccepted)
}

func respond(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)
}

func respondEncodeJson(w http.ResponseWriter, status int, data map[string]interface{}) {
	j, err := json.Marshal(data)
	if err != nil {
		respondWith500(w, "json marshaling failed")
		return
	}
	respond(w, status, j)
}

func respondWith500(w http.ResponseWriter, errMsg string) {
	errJson := fmt.Sprintf(`{"error":"%v"}`, strings.Replace(errMsg, `"`, `\"`, -1))
	respond(w, http.StatusInternalServerError, []byte(errJson))
}
