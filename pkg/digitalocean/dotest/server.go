package dotest

import (
	"net/http"

	"github.com/gorilla/mux"
)

var (
	DeniedResponse   = &response{501, `{"status":"ERROR","message":"Access Denied"}`}
	NotfoundResponse = &response{404, `{"status":"ERROR","message":"Not Found"}`}
	OkResponse       = &response{200, `{"status":"OK"}`}

	urlMapping = map[string]string{
		"/droplets":                         "droplets",
		"/droplets/new":                     "droplets/new",
		"/droplets/{id:[0-9]+}":             "droplets/id",
		"/droplets/{id:[0-9]+}/destroy":     "droplets/id/destroy",
		"/droplets/{id:[0-9]+}/power_off":   "droplets/id/power_off",
		"/droplets/{id:[0-9]+}/power_on":    "droplets/id/power_on",
		"/droplets/{id:[0-9]+}/power_cycle": "droplets/id/power_cycle",
		"/droplets/{id:[0-9]+}/reboot":      "droplets/id/reboot",
		"/droplets/{id:[0-9]+}/snapshot":    "droplets/id/snapshot",
		"/droplets/{id:[0-9]+}/restore":     "droplets/id/restore",
		"/sizes":                            "sizes",
		"/regions":                          "regions",
		"/images":                           "images",
		"/images/{id}":                      "images/id",
		"/ssh_keys/new":                     "ssh_keys/new",
		"/ssh_keys/{id}/destroy":            "ssh_keys/id/destroy",
		"/events/{id}":                      "events/id",
	}
)

// return the json string of the response
type responder func(w http.ResponseWriter, r *http.Request) *response

type response struct {
	code int
	data string
}

type Server struct {
	Router    *mux.Router
	Responses map[string][]*response
	Requests  map[string][]*http.Request
	rCounter  map[string]int
}

func (s *Server) makeResponse(endpoint string) func(w http.ResponseWriter, r *http.Request) {
	cachedEndpoint := endpoint
	return func(w http.ResponseWriter, r *http.Request) {
		s.addRequests(cachedEndpoint, r)
		jsonData := s.nextResponse(cachedEndpoint)
		if jsonData.code == 0 {
			jsonData.code = 200
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(jsonData.code)
		w.Write([]byte(jsonData.data))
	}
}

func (s *Server) nextResponse(endpoint string) *response {
	responses, ok := s.Responses[endpoint]
	if !ok {
		return NotfoundResponse
	}
	c, ok := s.rCounter[endpoint]
	if !ok {
		s.rCounter[endpoint] = 0
	}
	if len(responses) <= int(c) {
		return NotfoundResponse
	}
	r := responses[c]
	s.rCounter[endpoint] += 1
	return r
}

func (s *Server) addRequests(endpoint string, r *http.Request) {
	_, ok := s.Requests[endpoint]
	if ok {
		s.Requests[endpoint] = append(s.Requests[endpoint], r)
	} else {
		s.Requests[endpoint] = []*http.Request{r}
	}
}

func (s *Server) AddResponse(endpoint string, r *response) {
	_, ok := s.Responses[endpoint]
	if ok {
		s.Responses[endpoint] = append(s.Responses[endpoint], r)
	} else {
		s.Responses[endpoint] = []*response{r}
	}
	_, ok = s.rCounter[endpoint]
	if !ok {
		s.rCounter[endpoint] = 0
	}
}

func NewServer() *Server {
	s := &Server{
		Responses: map[string][]*response{},
		Requests:  map[string][]*http.Request{},
		rCounter:  map[string]int{},
	}
	r := mux.NewRouter()
	for route, name := range urlMapping {
		r.HandleFunc(route, s.makeResponse(name))
	}
	s.Router = r
	return s

}

func NewResponse(code int, message string) *response {
	return &response{code, message}
}
