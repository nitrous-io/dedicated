package hostbuilder

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/pkg/uuid"
	"github.com/nitrous-io/nitrous-common/flusher"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/librato"
)

// form param that contains request uuid
const NitrousUUIDHeader = "X-NITROUS-UUID"

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
		30 * time.Second,
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
	StatusCode     int        `json:"-"`
	Status         string     `json:"status,omitempty"`
	Provider       string     `json:"provider,omitempty"`
	ID             string     `json:"id,omitempty"`
	Name           string     `json:"name,omitempty"`
	IPAddress      string     `json:"ip_address,omitempty"`
	FreighterdPort uint16     `json:"freighterd_port,omitempty"`
	Region         string     `json:"region_id,omitempty"`
	State          string     `json:"state,omitempty"`
	Stream         string     `json:"stream,omitempty"`
	Error          string     `json:"error,omitempty"`
	LaunchTime     *time.Time `json:"launch_time,omitempty"`
}

type HttpHandler func(w http.ResponseWriter, r *http.Request) error

func makeHttpHandler(handlerFunc HttpHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuidStr := r.Header.Get(NitrousUUIDHeader)
		if uuidStr == "" {
			uuidStr = uuid.Make()
			r.Header.Set(NitrousUUIDHeader, uuidStr)
		}

		start := time.Now()

		log.Printf("uuid=%s Request: %v %v\n", uuidStr, r.Method, r.URL.String())
		if err := handlerFunc(w, r); err != nil {
			duration := time.Since(start)
			log.Printf("uuid=%s [ERROR], Completed in %s: %s %s: %s\n", uuidStr, duration, r.Method, r.URL, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			duration := time.Since(start)
			log.Printf("uuid=%s 200 OK Completed in %s: %s %s\n", uuidStr, duration, r.Method, r.URL.String())
		}
	}
}

type providerConfig map[string]string

type RedisConfig struct {
	Host string `json:"host"`
	Port string `json:"port"`
	DB   int64  `json:"database"`
}

type config struct {
	Address   string                    `json:"address"`
	Providers map[string]providerConfig `json:"providers"`

	// librato settings
	LibratoUser  string `json:"librato_user"`
	LibratoToken string `json:"librato_token"`

	// max time to wait for graceful stop, in seconds
	StopTimeout int `json:"stop_timeout"`

	// start the server in debug mode
	Debug bool `json:"debug"`

	// redis setting
	RedisConfig RedisConfig `json:"redis"`

	// whether partial provider List() calls are considered an error
	AllowPartialList bool `json:"allow_partial_list"`
}

type Server struct {
	Config *config
	Router *mux.Router

	initQueueOnce sync.Once
	Queue         queue.Publisher
}

// Return a copy of the original provider config
// for safe local mutation
func (s *Server) providerConfig(provider string) map[string]string {
	config := s.Config.Providers[provider]
	configCopy := make(map[string]string)
	for k, v := range config {
		configCopy[k] = v
	}
	return configCopy
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
	var err error
	if err = r.ParseForm(); err != nil {
		return err
	}

	provider := r.Form.Get("provider")
	providerConfig := s.providerConfig(provider)
	providerConfig["uuid"] = r.Header.Get(NitrousUUIDHeader)

	configJson := r.Form.Get("config")
	log.Printf("uuid=%s Creating a host using provider %s", providerConfig["uuid"], provider)

	h, err := host.InitHost(provider, providerConfig, configJson)
	if err != nil {
		return mh.Error(err, fmt.Sprintf("create.%s", provider))
	}

	// record total time taken for this function
	label := fmt.Sprintf("create.%s", provider)
	defer mh.UpdateSince(label, time.Now())
	pw := NewProgressWriter(flusher.NewWriteFlusher(w))

	var info *host.Info
	if r.Form.Get("queue") == "true" {
		s.initQueueOnce.Do(func() {
			err = s.initQueue()
		})
		if err != nil {
			return mh.Error(err, fmt.Sprintf("create.%s", provider))
		}
		log.Printf("uuid=%s Creating a host with queue %+v", providerConfig["uuid"], s.Queue)

		var queueInfo *host.QueueInfo
		queueInfo, err = getQueueInfo(r)
		if err != nil {
			return mh.Error(err, fmt.Sprintf("create.%s", provider))
		}

		info, err = h.CreateWithQueue(s.Queue, queueInfo)
	} else {
		info, err = h.Create(pw)
	}

	// include any information about the host
	// even if an error occurs
	baseResp := &Response{}
	if info != nil {
		baseResp.StatusCode = http.StatusCreated
		baseResp.Status = "created"
		baseResp.Provider = info.Provider
		baseResp.ID = info.ID
		baseResp.Name = info.Name
		baseResp.IPAddress = info.IPAddress
		baseResp.FreighterdPort = info.FreighterdPort
		baseResp.Region = info.Region
		baseResp.State = info.State.String()
	}

	if err != nil {
		mh.Error(err, label)
		log.Printf("uuid=%s Failed to create host, err: %v", providerConfig["uuid"], err)
		baseResp.Error = fmt.Sprintf("failed to create a host on %s", provider)
		pw.WriteResponse(baseResp)
		return nil
	}
	log.Printf("uuid=%s Created host: %+v", providerConfig["uuid"], info)

	mh.Time(fmt.Sprintf("ping.%s", label), func() {
		// do not remove a host if freighter is down, we keep the host for debugging
		pingHost(info, pw)
	})

	pw.WriteResponse(baseResp)

	return nil
}

func (s *Server) updateHost(w http.ResponseWriter, r *http.Request) error {
	h, err := s.hostFromRequest(r, nil)
	if err != nil {
		label := fmt.Sprintf("update.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	label := fmt.Sprintf("update.%s", h.DriverName())
	defer mh.UpdateSince(label, time.Now())

	pw := NewProgressWriter(flusher.NewWriteFlusher(w))

	info, err := h.Update(pw)
	if err != nil {
		pw.WriteError(fmt.Errorf("failed to update host on %s, err: %v", h.DriverName(), err))
		return nil
	}

	if err := pingHost(info, pw); err != nil {
		pw.WriteError(fmt.Errorf("failed to ping host on %s after updating, err: %v", h.DriverName(), err))
		return nil
	}

	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "updated",
		Provider:       info.Provider,
		ID:             info.ID,
		Name:           info.Name,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
		Region:         info.Region,
		State:          info.State.String(),
	}
	pw.WriteResponse(resp)
	return nil
}

func (s *Server) respecHost(w http.ResponseWriter, r *http.Request) error {
	h, err := s.hostFromRequest(r, nil)
	if err != nil {
		label := fmt.Sprintf("respec.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	label := fmt.Sprintf("respec.%s", h.DriverName())
	defer mh.UpdateSince(label, time.Now())

	info, err := h.Respec()
	if err != nil {
		return mh.Error(err, label)
	}

	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "respecced",
		Provider:       info.Provider,
		ID:             info.ID,
		Name:           info.Name,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
		Region:         info.Region,
		State:          info.State.String(),
	}
	writeResponse(w, resp)
	return nil
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) error {
	h, err := s.hostFromRequest(r, nil)
	if err != nil {
		label := fmt.Sprintf("delete.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	// record total time taken for this function
	label := fmt.Sprintf("delete.%s", h.DriverName())
	defer mh.UpdateSince(label, time.Now())

	if err := h.Destroy(); err != nil {
		return mh.Error(err, label)
	}
	writeResponse(w, okResponse)
	return nil
}

func (s *Server) stopHost(w http.ResponseWriter, r *http.Request) error {
	var err error
	var info *host.Info

	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.providerConfig(provider)
	configJson := r.Form.Get("config")

	h, err := host.InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}

	if r.Form.Get("queue") == "true" {
		s.initQueueOnce.Do(func() {
			err = s.initQueue()
		})
		if err != nil {
			return mh.Error(err, fmt.Sprintf("create.%s", provider))
		}
		log.Printf("uuid=%s Stopping a host with queue %+v\n", providerConfig["uuid"], *s.Queue)

		var queueInfo *host.QueueInfo
		queueInfo, err = getQueueInfo(r)
		if err != nil {
			return mh.Error(err, fmt.Sprintf("create.%s", provider))
		}

		info, err = h.StopWithQueue(s.Queue, queueInfo)
	} else {
		info, err = h.Stop()
	}

	if err != nil {
		return err
	}

	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "stopped",
		Provider:       info.Provider,
		ID:             info.ID,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
		Region:         info.Region,
	}
	writeResponse(w, resp)
	return nil
}

func (s *Server) startHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.providerConfig(provider)
	configJson := r.Form.Get("config")

	h, err := host.InitHost(provider, providerConfig, configJson)
	if err != nil {
		return mh.Error(err, fmt.Sprintf("start.%s", provider))
	}

	label := fmt.Sprintf("start.%s", h.DriverName())
	defer mh.UpdateSince(label, time.Now())

	info, err := h.Start()
	if err != nil {
		return mh.Error(err, label)
	}

	t := mh.Timer(fmt.Sprintf("ping.%s", label))
	pingStart := time.Now()
	if err := pingHost(info, ioutil.Discard); err != nil {
		return mh.Error(err, fmt.Sprintf("ping.%s", label))
	}
	t.UpdateSince(pingStart)

	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "started",
		Provider:       info.Provider,
		ID:             info.ID,
		Name:           info.Name,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
		Region:         info.Region,
		State:          info.State.String(),
	}
	writeResponse(w, resp)
	return nil
}

func (s *Server) restartHost(w http.ResponseWriter, r *http.Request) error {
	h, err := s.hostFromRequest(r, nil)
	if err != nil {
		label := fmt.Sprintf("restart.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	// record total time taken for this function
	label := fmt.Sprintf("restart.%s", h.DriverName())
	defer mh.UpdateSince(label, time.Now())

	info, err := h.Restart()
	if err != nil {
		return mh.Error(err, label)
	}

	// ping the host to make sure that it's up
	t := mh.Timer(fmt.Sprintf("ping.%s", label))
	pingStart := time.Now()
	if err := pingHost(info, ioutil.Discard); err != nil {
		return mh.Error(err, fmt.Sprintf("ping.%s", label))
	}
	t.UpdateSince(pingStart)

	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "restarted",
		Provider:       info.Provider,
		ID:             info.ID,
		Name:           info.Name,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
		Region:         info.Region,
		State:          info.State.String(),
	}
	writeResponse(w, resp)
	return nil
}

// sleep waits for a defined seconds and then finishes the request
// This is useful for testing graceful shutdown and should only be enabled in
// debug mode
func (s *Server) sleep(w http.ResponseWriter, r *http.Request) (err error) {
	if !s.Config.Debug {
		return nil
	}
	var duration = 60 // duration in seconds
	r.URL.Query()
	if d := r.FormValue("duration"); d != "" {
		duration, err = strconv.Atoi(d)
		if err != nil {
			return err
		}
	}
	log.Printf("/test/sleep - sleeping for %d seconds", duration)
	time.Sleep(time.Duration(duration) * time.Second)
	fmt.Fprintf(w, "sleep for %d seconds OK", duration)
	return nil
}

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) error {
	// TODO This is independent of any particular host so we don't need to use
	// host-specific form data.
	h, err := s.hostFromRequest(r, map[string]string{"ignore_region": "true"})
	if err != nil {
		label := fmt.Sprintf("list.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	label := fmt.Sprintf("list.%s", h.DriverName())
	defer mh.UpdateSince(label, time.Now())

	hosts, partialErr := h.List()
	if partialErr != nil {
		if len(hosts) == 0 || !s.Config.AllowPartialList {
			return mh.Error(partialErr, label)
		}
	}

	responses := make([]Response, 0, len(hosts))
	for _, hst := range hosts {
		responses = append(responses, Response{
			Provider:       hst.Provider,
			ID:             hst.ID,
			Name:           hst.Name,
			IPAddress:      hst.IPAddress,
			FreighterdPort: hst.FreighterdPort,
			Region:         hst.Region,
			State:          hst.State.String(),
			LaunchTime:     hst.LaunchTime,
		})
	}

	dataMap := map[string]interface{}{
		"hosts": responses,
	}
	if partialErr != nil {
		dataMap["partial_error"] = partialErr.Error()
	}

	data, err := json.Marshal(dataMap)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		data = []byte(`{"status":"error"}`)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
	return nil
}

func (s *Server) hostFromRequest(r *http.Request, customConfig map[string]string) (host.Host, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.providerConfig(provider)
	for k, v := range customConfig {
		providerConfig[k] = v
	}
	providerConfig["uuid"] = r.Header.Get(NitrousUUIDHeader)
	configJSON := r.Form.Get("config")
	if configJSON == "" {
		configJSON = "{}"
	}
	return host.InitHost(provider, providerConfig, configJSON)
}

func (s *Server) CreateRouter() {
	r := mux.NewRouter()
	m := map[string]map[string]HttpHandler{
		"GET": {
			"/hosts": s.listHosts,
		},
		"POST": {
			"/hosts":         s.createHost,
			"/hosts/update":  s.updateHost,
			"/hosts/respec":  s.respecHost,
			"/hosts/delete":  s.deleteHost,
			"/hosts/stop":    s.stopHost,
			"/hosts/start":   s.startHost,
			"/hosts/restart": s.restartHost,
		},
	}
	if s.Config.Debug {
		m["GET"]["/test/sleep"] = s.sleep
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

	if config.LibratoUser == "" || config.LibratoToken == "" {
		log.Print("missing librato config. Metrics logging is disabled")
		// in dev, we flush metrics every 60s into devNull
		// because we are not interested in them
		go metrics.Log(metrics.DefaultRegistry, 60e9,
			log.New(ioutil.Discard, "hostbuilder: metrics: ", log.Lmicroseconds))
	} else {
		go librato.Librato(metrics.DefaultRegistry,
			30e9,                // interval
			config.LibratoUser,  // account owner email address
			config.LibratoToken, // Librato API token
			"hostbuilder",       // source
			[]float64{0.99},     // precentiles to send
			time.Millisecond,    // time unit
		)
	}

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

func (s *Server) initQueue() error {
	rc := s.Config.RedisConfig
	if rc.Host == "" || rc.Port == "" {
		return fmt.Errorf("Redis config is not valid: %+v", rc)
	}
	qp := queue.NewQueue(rc.Host, rc.Port, rc.DB, "v1.host")
	s.Queue = qp
	return nil
}

func getQueueInfo(r *http.Request) (*host.QueueInfo, error) {
	var userID, hostID int64
	hostSlug := r.Form.Get("slug")
	hostID, err := strconv.ParseInt(r.Form.Get("id"), 10, 64)
	if err != nil {
		return nil, err
	}

	userID, err = strconv.ParseInt(r.Form.Get("uid"), 10, 64)
	if err != nil {
		return nil, err
	}
	return &host.QueueInfo{ID: hostID, UserID: userID, Slug: hostSlug}, nil
}
