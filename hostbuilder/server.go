package hostbuilder

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
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

type httpHandler func(w http.ResponseWriter, r *http.Request) error

func makeHttpHandler(handlerFunc httpHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuidStr := r.Header.Get(NitrousUUIDHeader)
		if uuidStr == "" {
			uuidStr = uuid.Make()
			r.Header.Set(NitrousUUIDHeader, uuidStr)
		}

		start := time.Now()
		var err error
		if err = r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		logrus.Infof("uuid=%s Request: %v %v", uuidStr, r.Method, r.URL.String())
		if err := handlerFunc(w, r); err != nil {
			duration := time.Since(start)
			logrus.Errorf("uuid=%s [ERROR], Completed in %s: %s %s: %s", uuidStr, duration, r.Method, r.URL, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			duration := time.Since(start)
			logrus.Infof("uuid=%s 200 OK Completed in %s: %s %s", uuidStr, duration, r.Method, r.URL.String())
		}
	}
}

type Server struct {
	Config *Config
	Router *mux.Router

	Queue queue.Publisher
}

// Return a copy of the original provider config for safe local mutation.
func (s *Server) providerConfig(provider string) map[string]interface{} {
	cp := make(map[string]interface{})
	for k, v := range s.Config.Providers[provider] {
		cp[k] = v
	}
	return cp
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
	driver, err := s.driverFromRequest(r, nil)
	if err != nil {
		return mh.Error(err, fmt.Sprintf("create.%s", r.Form.Get("provider")))
	}

	pw := NewProgressWriter(flusher.NewWriteFlusher(w))
	requestID := r.Header.Get(NitrousUUIDHeader)

	logrus.Infof("uuid=%s Creating a host using provider %s", requestID, driver.DriverName())

	label := fmt.Sprintf("create.%s", driver.DriverName())
	defer mh.UpdateSince(label, time.Now())

	queueInfo, err := getQueueInfo(r)
	if err != nil {
		return mh.Error(err, fmt.Sprintf("create.%s", driver.DriverName()))
	}

	info, err := driver.CreateWithQueue(s.Queue, queueInfo)
	// include any information about the host even if an error occurs
	resp := &Response{}
	if info != nil {
		resp.StatusCode = http.StatusCreated
		resp.Status = "created"
		resp.Provider = info.Provider
		resp.ID = info.ID
		resp.Name = info.Name
		resp.IPAddress = info.IPAddress
		resp.FreighterdPort = info.FreighterdPort
		resp.Region = info.Region
		resp.State = info.State.String()
	}
	if err != nil {
		mh.Error(err, label)
		logrus.Errorf("uuid=%s Failed to create host, err: %v", requestID, err)
		resp.Error = fmt.Sprintf("failed to create a host on %s", driver.DriverName())
		pw.WriteResponse(resp)
		return nil
	}
	logrus.Infof("uuid=%s Created host: %+v", requestID, info)

	mh.Time(fmt.Sprintf("ping.%s", label), func() {
		// do not remove a host if freighter is down, we keep the host for debugging
		pingHost(info, pw)
	})

	pw.WriteResponse(resp)

	return nil
}

func (s *Server) updateHost(w http.ResponseWriter, r *http.Request) error {
	driver, err := s.driverFromRequest(r, nil)
	if err != nil {
		label := fmt.Sprintf("update.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	label := fmt.Sprintf("update.%s", driver.DriverName())
	defer mh.UpdateSince(label, time.Now())

	pw := NewProgressWriter(flusher.NewWriteFlusher(w))

	info, err := driver.Update(pw)
	if err != nil {
		pw.WriteError(fmt.Errorf("failed to update host on %s, err: %v", driver.DriverName(), err))
		return nil
	}

	if err := pingHost(info, pw); err != nil {
		pw.WriteError(fmt.Errorf("failed to ping host on %s after updating, err: %v", driver.DriverName(), err))
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
	driver, err := s.driverFromRequest(r, nil)
	if err != nil {
		label := fmt.Sprintf("respec.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	label := fmt.Sprintf("respec.%s", driver.DriverName())
	defer mh.UpdateSince(label, time.Now())

	info, err := driver.Respec()
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
	driver, err := s.driverFromRequest(r, nil)
	if err != nil {
		label := fmt.Sprintf("delete.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	// record total time taken for this function
	label := fmt.Sprintf("delete.%s", driver.DriverName())
	defer mh.UpdateSince(label, time.Now())

	if err := driver.Destroy(); err != nil {
		return mh.Error(err, label)
	}
	writeResponse(w, okResponse)
	return nil
}

func (s *Server) stopHost(w http.ResponseWriter, r *http.Request) error {
	driver, err := s.driverFromRequest(r, nil)
	if err != nil {
		return err
	}

	var queueInfo *host.QueueInfo
	queueInfo, err = getQueueInfo(r)
	if err != nil {
		return mh.Error(err, fmt.Sprintf("create.%s", driver.DriverName()))
	}

	info, err := driver.StopWithQueue(s.Queue, queueInfo)
	if err != nil {
		logrus.Errorf("uuid=%s Failed to stop a host: %v", r.Header.Get(NitrousUUIDHeader), err)
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
	driver, err := s.driverFromRequest(r, nil)
	if err != nil {
		return mh.Error(err, fmt.Sprintf("start.%s", r.Form.Get("provider")))
	}

	label := fmt.Sprintf("start.%s", driver.DriverName())
	defer mh.UpdateSince(label, time.Now())

	info, err := driver.Start()
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
	driver, err := s.driverFromRequest(r, nil)
	if err != nil {
		label := fmt.Sprintf("restart.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	// record total time taken for this function
	label := fmt.Sprintf("restart.%s", driver.DriverName())
	defer mh.UpdateSince(label, time.Now())

	info, err := driver.Restart()
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
	logrus.Debugf("/test/sleep - sleeping for %d seconds", duration)
	time.Sleep(time.Duration(duration) * time.Second)
	fmt.Fprintf(w, "sleep for %d seconds OK", duration)
	return nil
}

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) error {
	// TODO This is independent of any particular host so we don't need to use
	// host-specific form data.
	driver, err := s.driverFromRequest(r, map[string]string{"ignore_region": "true"})
	if err != nil {
		label := fmt.Sprintf("list.%s", r.Form.Get("provider"))
		return mh.Error(err, label)
	}

	label := fmt.Sprintf("list.%s", driver.DriverName())
	defer mh.UpdateSince(label, time.Now())

	hosts, partialErr := driver.List()
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

func (s *Server) driverFromRequest(r *http.Request, customConfig map[string]string) (host.Host, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	provider := r.Form.Get("provider")
	providerConfig := s.providerConfig(provider)
	for k, v := range customConfig {
		providerConfig[k] = v
	}
	b, err := json.Marshal(providerConfig)
	if err != nil {
		return nil, err
	}

	hostConfig := r.Form.Get("config")
	if hostConfig == "" {
		hostConfig = "{}"
	}

	driver, err := host.NewDriver(provider, string(b), hostConfig)
	if err != nil {
		return nil, err
	}

	driver.SetRequestID(r.Header.Get(NitrousUUIDHeader))

	return driver, nil
}

func (s *Server) createRouter() {
	r := mux.NewRouter()
	m := map[string]map[string]httpHandler{
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

func New(config *Config) (*Server, error) {
	server := &Server{Config: config}
	server.createRouter()
	server.addDebugRoutes(server.Router)

	if config.LibratoUser == "" || config.LibratoToken == "" {
		logrus.Warn("missing librato config. Metrics logging is disabled")
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

	// Initialize queue client
	if err := server.initQueue(); err != nil {
		logrus.Errorf("hostbuilder: Failed to initialize queue: %+v", err)
		return nil, err
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
	if s.Config.QueueType == "rabbitmq" {
		qp, err := queue.NewAmqpQueue(s.Config.RabbitMQAddr, "v1.host")
		if err != nil {
			return err
		}
		s.Queue = qp
		return nil
	}
	rc := s.Config.RedisConfig
	if rc.Host == "" || rc.Port == "" {
		return fmt.Errorf("Redis config is not valid: %+v", rc)
	}
	qp := queue.NewQueue(rc.Host, rc.Port, rc.DB, "v1.host")
	s.Queue = qp
	return nil
}

func getQueueInfo(r *http.Request) (*host.QueueInfo, error) {
	var userID, hostID, requesterID int64
	hostSlug := r.Form.Get("slug")
	hostID, err := strconv.ParseInt(r.Form.Get("id"), 10, 64)
	if err != nil {
		return nil, err
	}

	userID, err = strconv.ParseInt(r.Form.Get("uid"), 10, 64)
	if err != nil {
		return nil, err
	}

	// FIXME: requester_id can be nil from old nitrousd.
	// Once we successfully release this, we can have error check here.
	requesterID, _ = strconv.ParseInt(r.Form.Get("requester_id"), 10, 64)
	return &host.QueueInfo{ID: hostID, UserID: userID, Slug: hostSlug, RequesterID: requesterID}, nil
}
