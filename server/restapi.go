package server

import (
	"net/http"

	"github.com/nitrous-io/freighter/provider"
)

type RestAPI struct {
	Provider provider.Provider
}

func (a *RestAPI) GetHome(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"status":"ok"}`))
}

func (a *RestAPI) PostContainers(w http.ResponseWriter, r *http.Request) {
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
