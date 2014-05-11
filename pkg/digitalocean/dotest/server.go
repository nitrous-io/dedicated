package dotest

import (
	"net/http"

	"github.com/gorilla/mux"
)

var (
	DeniedResponse   = &response{501, `{"status":"ERROR","message":"Access Denied"}`}
	NotfoundResponse = &response{404, `{"status":"ERROR","message":"Not Found"}`}

	urlMapping = map[string]string{
		"/droplets":      "droplets",
		"/droplets/new":  "droplets/new",
		"/droplets/{id}": "droplets/id",
		"/sizes":         "sizes",
		"/regions":       "regions",
		"/images":        "images",
		"/images/{id}":   "images/id",
	}
)

// return the json string of the response
type responder func(w http.ResponseWriter, r *http.Request) *response

type response struct {
	code int
	data string
}

type responses struct {
	list []*response
	c    counter
}

func (r *responses) next() *response {
	if len(r.list) <= int(r.c) {
		return nil
	}
	res := r.list[r.c]
	r.c.next()
	return res
}

type Server struct {
	Router    *mux.Router
	Responses map[string]*responses
	Requests  map[string][]*http.Request
}

func makeResponse(s *Server, endpoint string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		s.addRequests(endpoint, r)
		jsonData := s.Responses[endpoint].next()
		if jsonData == nil {
			jsonData = NotfoundResponse
		}
		if jsonData.code == 0 {
			jsonData.code = 200
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(jsonData.code)
		w.Write([]byte(jsonData.data))
	}
}

func (s *Server) addRequests(endpoint string, r *http.Request) {
	if _, ok := s.Requests[endpoint]; !ok {
		s.Requests[endpoint] = []*http.Request{}
	}
	s.Requests[endpoint] = append(s.Requests[endpoint], r)
}

func (s *Server) AddResponse(endpoint string, r *response) {
	if _, ok := s.Responses[endpoint]; !ok {
		s.Responses[endpoint] = &responses{list: []*response{}, c: 0}
	}
	ep := s.Responses[endpoint]
	ep.list = append(ep.list, r)
}

func NewServer() *Server {
	s := &Server{
		Responses: map[string]*responses{},
		Requests:  map[string][]*http.Request{},
	}
	r := mux.NewRouter()
	for route, name := range urlMapping {
		r.HandleFunc(route, makeResponse(s, name))
	}
	s.Router = r
	return s

}

func NewResponse(code int, message string) *response {
	return &response{code, message}
}
