package hostbuilder

import (
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
		}
	}
}

type providerConfig map[string]string

type config struct {
	Address   string                    `json:"address"`
	Providers map[string]providerConfig `json:"providers"`
}

type WebhookData struct {
	Success bool   `json:"success"`
	Message string `json:"message"`

	Provider  string `json:"provider"`
	Id        string `json:"id"`
	IpAddress string `json:"ip_address"`
}

type Server struct {
	Config *config
	Router *mux.Router
}

func doWebHook(endpoint string, data string) {
	params := url.Values{}
	params.Set("payload", data)
	http.PostForm(endpoint, params)
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
	err = h.Create()
	go func() {
		log.Println("performing webhook for create")
		d := &WebhookData{}
		var info *host.Info
		if err == nil {
			info, err = h.Info()
		}
		if err != nil {
			d.Success = false
			d.Message = fmt.Sprintf("Error: %v", err)
		} else {
			d.Success = true
			d.Provider = info.Provider
			d.Id = info.Id
			d.IpAddress = info.IpAddress
		}
		b, _ := json.Marshal(d)
		doWebHook(webhook, string(b))
	}()
	return err
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
	err = h.Destroy()
	go func() {
		log.Println("performing webhook for destroy")
		d := &WebhookData{}
		if err != nil {
			d.Success = false
			d.Message = fmt.Sprintf("Error: %v", err)
		} else {
			d.Success = true
		}
		b, _ := json.Marshal(d)
		doWebHook(webhook, string(b))
	}()
	return err
}

func (s *Server) CreateRouter() {
	r := mux.NewRouter()
	m := map[string]map[string]HttpHandler{
		"POST": {
			"/host":        s.postHost,
			"/host/delete": s.deleteHost,
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
