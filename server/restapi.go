package server

import (
	"net/http"
)

type RestAPI struct{}

func (a *RestAPI) Home(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"status":"ok"}`))
}
