package hostbuilder

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/hostbuilder/host"
)

type HttpHandler func(w http.ResponseWriter, r *http.Request) error

func makeHttpHandler(handlerFunc HttpHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := handlerFunc(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		}
	}
}

type providerConfig map[string]string

type config struct {
	Address   string                    `json:"address"`
	Providers map[string]providerConfig `json:"providers"`
}

type WebhookData struct {
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
}

type Server struct {
	Config *config
	Router *mux.Router
}

func doWebhook(endpoint string, f func() (string, error)) {
	d := &WebhookData{Success: true}
	msg, err := f()
	if err != nil {
		d.Success = false
		d.Message = fmt.Sprintf("Error: %v", err)
	} else {
		d.Message = msg
	}
	payload, _ := json.Marshal(d)
	params := url.Values{}
	params.Set("payload", string(payload))
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	// if debug mode is on (DEBUG=true env) then we skip cert verification
	if debug {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Transport: transport}
	// TODO: retry the webhook if it failes for a few times
	_, err = client.PostForm(endpoint, params)
	if err != nil {
		log.Printf("hostbuilder: error posting to webhook: %v", err)
	}
}

func toString(b []byte, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Server) postHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	webhook := r.Form.Get("webhook")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	go doWebhook(webhook, func() (string, error) {
		var info *host.Info
		info, err = h.Create()
		log.Printf("Created: %+v\n, %v", info, err)
		if err != nil {
			return "", err
		}
		return toString(json.Marshal(info))
	})
	return nil
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	webhook := r.Form.Get("webhook")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	go doWebhook(webhook, func() (string, error) {
		return "", h.Destroy()
	})
	return nil
}

func (s *Server) stopHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	webhook := r.Form.Get("webhook")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	go doWebhook(webhook, func() (string, error) {
		return "", h.Stop()
	})
	return nil
}

func (s *Server) startHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	webhook := r.Form.Get("webhook")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	go doWebhook(webhook, func() (string, error) {
		return "", h.Start()
	})
	return nil
}

func (s *Server) restartHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	webhook := r.Form.Get("webhook")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	go doWebhook(webhook, func() (string, error) {
		return "", h.Restart()
	})
	return nil
}

func (s *Server) CreateRouter() {
	r := mux.NewRouter()
	m := map[string]map[string]HttpHandler{
		"POST": {
			"/host":         s.postHost,
			"/host/delete":  s.deleteHost,
			"/host/stop":    s.stopHost,
			"/host/start":   s.startHost,
			"/host/restart": s.restartHost,
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
