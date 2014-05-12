package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
)

type RestAPI struct {
	Provider provider.Provider
	Callback Callback
}

func (a *RestAPI) Home(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, []byte(`{"status":"ok"}`))
}

func (a *RestAPI) ListContainers(w http.ResponseWriter, r *http.Request) {
	infoList, err := a.Provider.List()
	if err != nil {
		respondWith500(w, "failed to fetch container list")
		return
	}

	j, err := json.Marshal(map[string][]provider.ContainerInfo{"containers": infoList})
	if err != nil {
		respondWith500(w, "json marshaling failed")
		return
	}

	respond(w, http.StatusOK, j)
}

func (a *RestAPI) CreateContainer(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	callbackUrl := r.FormValue("callback_url")
	publicKeys := r.PostForm["public_key[]"]

	if name == "" || callbackUrl == "" || publicKeys == nil {
		respond(w, 422, []byte(`{"error":"invalid params"}`))
		return
	}

	go func() {
		info, err := a.Provider.Create(name, publicKeys)
		if err != nil {
			payload, err := json.Marshal(map[string]string{"error": err.Error()})
			if err == nil {
				a.Callback.Invoke(callbackUrl, string(payload))
			}
			return
		}
		payload, err := json.Marshal(map[string]interface{}{"status": "created", "container": info})
		if err == nil {
			a.Callback.Invoke(callbackUrl, string(payload))
		}
	}()

	respond(w, http.StatusAccepted, []byte(`{"status":"accepted"}`))
}

func (a *RestAPI) GetContainerInfo(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	info, err := a.Provider.GetInfo(name)

	if err != nil {
		respond(w, http.StatusNotFound, []byte(`{"error":"not found"}`))
		return
	}

	j, err := json.Marshal(map[string]*provider.ContainerInfo{"container": info})
	if err != nil {
		respondWith500(w, "json marshaling failed")
		return
	}

	respond(w, http.StatusOK, j)
}

func respond(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)
}

func respondWith500(w http.ResponseWriter, errMsg string) {
	errJson := fmt.Sprintf(`{"error":"%v"}`, strings.Replace(errMsg, `"`, `\"`, -1))
	respond(w, http.StatusInternalServerError, []byte(errJson))
}
