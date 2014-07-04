package hostbuilder

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/hostbuilder/host"
)

type HttpHandler func(w http.ResponseWriter, r *http.Request) error

func makeHttpHandler(handlerFunc HttpHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := handlerFunc(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
}

type providerConfig map[string]string

type config struct {
	Address   string                    `json:"address"`
	Providers map[string]providerConfig `json:"providers"`
}

type Server struct {
	Config *config
	Router *mux.Router
}

func (s *Server) postHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}

	var info *host.Info
	info, err = h.Create()
	log.Printf("Created: %+v\n, %v", info, err)
	if err != nil {
		return err
	}
	j, err := json.Marshal(info)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	w.Write(j)
	return nil
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	err = h.Destroy()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
	return nil
}

func (s *Server) stopHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	err = h.Stop()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
	return nil
}

func (s *Server) startHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	err = h.Start()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
	return nil
}

func (s *Server) restartHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	err = h.Restart()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
	return nil
}

func (s *Server) CreateRouter() {
	r := mux.NewRouter()
	m := map[string]map[string]HttpHandler{
		"POST": {
			"/hosts":         s.postHost,
			"/hosts/delete":  s.deleteHost,
			"/hosts/stop":    s.stopHost,
			"/hosts/start":   s.startHost,
			"/hosts/restart": s.restartHost,
		},
	}
	for method, routes := range m {
		for route, handler := range routes {
			localRoute := route
			localHandler := handler
			localMethod := method
			f := makeHttpHandler(localHandler)
			r.HandleFunc(localRoute, f).Methods(localMethod)
		}
	}
	s.Router = r
}

func New(configData []byte) (*Server, error) {
	config := &config{}
	if err := json.Unmarshal(configData, config); err != nil {
		log.Printf("hostbuilder: invalid config data %v", string(configData))
		return nil, err
	}
	server := &Server{Config: config}
	server.CreateRouter()
	return server, nil
}
