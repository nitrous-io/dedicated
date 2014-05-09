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
}

func (a *RestAPI) Home(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"status":"ok"}`))
}

func (a *RestAPI) CreateContainer(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	webhookUrl := r.FormValue("webhook_url")
	publicKeys := r.PostForm["public_key[]"]

	if name == "" || webhookUrl == "" || publicKeys == nil {
		w.WriteHeader(422)
		w.Write([]byte(`{"error":"invalid params"}`))
		return
	}

	go func() {
		a.Provider.Create(name, publicKeys, webhookUrl)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}

func (a *RestAPI) GetContainerInfo(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	info, err := a.Provider.GetInfo(name)

	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
		return
	}

	j, err := json.Marshal(map[string]*provider.ContainerInfo{"container": info})
	if err != nil {
		respondWith500(w, "json marshaling failed")
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(j)
}

func respondWith500(w http.ResponseWriter, errMsg string) {
	w.WriteHeader(http.StatusInternalServerError)
	errJson := fmt.Sprintf(`{"error":"%v"}`, strings.Replace(errMsg, `"`, `\"`, -1))
	w.Write([]byte(errJson))
}
