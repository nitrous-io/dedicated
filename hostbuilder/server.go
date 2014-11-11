package hostbuilder

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/nitrous-common/flusher"
	"github.com/rcrowley/go-librato"
)

// form param that contains request uuid
const UUIDParam = "hb-uuid"

var (
	okResponse = &Response{StatusCode: http.StatusOK, Status: "ok"}

	FreighterPingTimeout  = 1 * time.Second
	FreighterPingAttempts = []time.Duration{
		2 * time.Second, // Ping eagerly at the start because most of the time
		2 * time.Second, // Freighter should be available pretty quickly.
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		15 * time.Second,
		20 * time.Second,
		30 * time.Second,
	}
)

// ProgressWriter write the stream data emitted by the host builder as new line
// separated json messages
type ProgressWriter struct {
	writer io.Writer
}

func NewProgressWriter(w io.Writer) *ProgressWriter {
	return &ProgressWriter{w}
}

func (w *ProgressWriter) Write(p []byte) (int, error) {
	err := w.WriteResponse(&Response{Stream: string(p)})
	return len(p), err
}

func (w *ProgressWriter) WriteError(err error) error {
	return w.WriteResponse(&Response{Error: err.Error()})
}

func (w *ProgressWriter) WriteResponse(r *Response) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w.writer, "%s\n", string(b))
	return err
}

// API response of the server
type Response struct {
	StatusCode     int    `json:"-"`
	Status         string `json:"status,omitempty"`
	Provider       string `json:"provider,omitempty"`
	ID             string `json:"id,omitempty"`
	IPAddress      string `json:"ip_address,omitempty"`
	FreighterdPort uint16 `json:"freighterd_port,omitempty"`
	Stream         string `json:"stream,omitempty"`
	Error          string `json:"error,omitempty"`
}

type HttpHandler func(w http.ResponseWriter, r *http.Request) error

func makeHttpHandler(handlerFunc HttpHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO use a pool instead of generating each time
		id := make([]byte, 4) // hex string len 8
		if _, err := io.ReadFull(rand.Reader, id); err != nil {
			panic(err)
		}
		requestID := hex.EncodeToString(id)
		if r.FormValue(UUIDParam) == "" {
			r.Form.Set(UUIDParam, requestID)
		}
		start := time.Now()

		log.Printf("uuid=%s Request: %v %v\n", requestID, r.Method, r.URL.String())
		if err := handlerFunc(w, r); err != nil {
			duration := time.Since(start)
			log.Printf("uuid=%s [ERROR], Completed in %s: %s %s: %s\n", requestID, duration, r.Method, r.URL, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			duration := time.Since(start)
			log.Printf("uuid=%s 200 OK Completed in %s: %s %s\n", requestID, duration, r.Method, r.URL.String())
		}
	}
}

type providerConfig map[string]string

type config struct {
	Address   string                    `json:"address"`
	Providers map[string]providerConfig `json:"providers"`

	// librato settings
	LibratoUser  string `json:"librato_user"`
	LibratoToken string `json:"librato_token"`
}

type Server struct {
	Config *config
	Router *mux.Router
}

// createHost streams progress data to the output. For example
//
// In success case:
// { "stream": "Creating key..." }
// { "stream": "Creating key...Done!" }
// { "stream": "Creating host..." }
// { "stream": "Creating host...Done!" }
// { "host": {"status": "created", "provider": "digitalocean" } }
//
// In error case:
// { "stream": "Creating key..." }
// { "stream": "Creating key...Done!" }
// { "stream": "Creating host..." }
// { "stream": "Creating host...Failed!" }
// { "error": "failed to create the host" }
func (s *Server) createHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	if providerConfig == nil {
		providerConfig = make(map[string]string)
	}

	if uuid := r.FormValue(UUIDParam); uuid != "" {
		providerConfig["uuid"] = uuid
	} else {
		// shouldn't happen, use this to detect if it does
		providerConfig["uuid"] = "NOUUID"
	}

	providerConfig["librato_user"] = s.Config.LibratoUser
	providerConfig["librato_token"] = s.Config.LibratoToken

	configJson := r.Form.Get("config")
	log.Printf("uuid=%s Creating a host using provider %s", providerConfig["uuid"], provider)

	m := librato.NewSimpleMetrics(s.Config.LibratoUser, s.Config.LibratoToken,
		fmt.Sprintf("hostbuilder_%s", provider))

	defer gaugeMetric(m, time.Now(), "create_host")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}

	pw := NewProgressWriter(flusher.NewWriteFlusher(w))

	info, err := h.Create(pw)
	log.Printf("uuid=%s Created: %+v\n, %v", providerConfig["uuid"], info, err)
	if err != nil {
		countMetric(m, 1, "create_host_error")
		pw.WriteError(fmt.Errorf("failed to create a host on %s", provider))
		return nil
	}
	// do not remove a host if freighter is down, we keep the host for debugging
	duration(m, "create_ping", func() {
		pingHost(info, pw)
	})
	resp := &Response{
		StatusCode:     http.StatusCreated,
		Status:         "created",
		Provider:       info.Provider,
		ID:             info.ID,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
	}
	pw.WriteResponse(resp)

	return nil
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	m := librato.NewSimpleMetrics(s.Config.LibratoUser, s.Config.LibratoToken,
		fmt.Sprintf("hostbuilder_%s", provider))

	defer gaugeMetric(m, time.Now(), "delete_host")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	err = h.Destroy()
	if err != nil {
		countMetric(m, 1, "delete_host_error")
		return err
	}
	writeResponse(w, okResponse)
	return nil
}

func (s *Server) restartHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	m := librato.NewSimpleMetrics(s.Config.LibratoUser, s.Config.LibratoToken,
		fmt.Sprintf("hostbuilder_%s", provider))

	defer gaugeMetric(m, time.Now(), "restart_host")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	info, err := h.Restart()
	if err != nil {
		countMetric(m, 1, "restart_host_error")
		return err
	}
	if err := pingHost(info, ioutil.Discard); err != nil {
		return err
	}
	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "restarted",
		Provider:       info.Provider,
		ID:             info.ID,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
	}
	writeResponse(w, resp)
	return nil
}

func (s *Server) CreateRouter() {
	r := mux.NewRouter()
	m := map[string]map[string]HttpHandler{
		"POST": {
			"/hosts":         s.createHost,
			"/hosts/delete":  s.deleteHost,
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

func writeResponse(w http.ResponseWriter, resp *Response) {
	w.Header().Set("Content-Type", "application/json")
	data, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		data = []byte(`{"status":"error"}`)
	} else {
		w.WriteHeader(resp.StatusCode)
	}
	w.Write(data)
}

// ping the host to make sure that it's up
func pingHost(info *host.Info, w io.Writer) error {
	u := fmt.Sprintf("https://%s:%d", info.IPAddress, info.FreighterdPort)
	fmt.Fprintf(w, "Connecting to freighter server at %s...", u)
	if !host.WaitForServer(u, FreighterPingTimeout, FreighterPingAttempts) {
		fmt.Fprintf(w, "Connecting to freighter server...Failed!")
		return fmt.Errorf("Connecting to freighter server...Failed!")
	}
	return nil
}

func duration(m librato.Metrics, name string, f func()) {
	g := m.GetGauge(name)
	defer func(start time.Time) {
		g <- int64(time.Since(start).Seconds() * 1000)
	}(time.Now())
	f()
}

func gaugeMetric(m librato.Metrics, start time.Time, name string) {
	g := m.GetGauge(name)
	g <- int64(time.Since(start).Seconds() * 1000)
}

func countMetric(m librato.Metrics, count int64, name string) {
	c := m.GetCounter(name)
	c <- count
}
