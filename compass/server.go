package compass

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	n "github.com/nitrous-io/compass/navigator"
)

type NavigatorConfig struct {
	Name   string            `json:"name"`
	Domain string            `json:"domain"` // which domain this navigator will handle
	Config map[string]string `json:"config"` // configuration of the navigator
}

type Config struct {
	Addr             string            `json:"address"` // host:port
	NavigatorConfigs []NavigatorConfig `json:"navigators"`
}

type Server struct {
	config     *Config
	db         *sql.DB
	mux        *dns.ServeMux
	wg         *sync.WaitGroup
	navigators map[string]n.Navigator
}

const (
	defaultTTL = uint32(300)
)

func (s *Server) ListenAndServe() error {
	s.mux = dns.NewServeMux()
	s.mux.Handle(".", s)

	s.wg.Add(2)
	go s.createServer("tcp", s.config.Addr)
	go s.createServer("udp", s.config.Addr)
	s.wg.Wait()
	return nil
}

// TODO: ideally we want this to
// - clean up the goroutine
// - exit the ListenAndServe routine
func (s *Server) Stop() {
}

func (s *Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	q := r.Question[0]
	log.Println("Question", q, q.Qtype, q.Name)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer = make([]dns.RR, 0, 10)

	// always write dns message back
	defer func() {
		if len(m.Answer) > 1 {
			for _, r := range m.Answer {
				r.Header().Ttl = uint32(defaultTTL)
			}
		}
		w.WriteMsg(m)
	}()

	// handle SOA
	navigator, ok := s.navigators[q.Name]
	if ok && q.Qtype == dns.TypeSOA {
		m.Answer = navigator.NewSOA()
		return
	}

	// handle DNS requests
	switch q.Qtype {
	case dns.TypeA:
		for domain, navigator := range s.navigators {
			// if the navigator handles this domain
			if strings.HasSuffix(q.Name, domain) {
				r, err := navigator.Lookup(q.Name)
				if err == nil {
					m.Answer = r
				} else {
					log.Printf("navigator %s failed to resolve %s, err %s", domain, q.Name, err)
				}
			}
		}
	}

	// if we cant handle the request, just return all our SOA as NS
	if len(m.Answer) == 0 {
		m.Ns = s.allSOA()
		m.Ns[0].Header().Ttl = defaultTTL
	}
}

func (s *Server) createServer(net, addr string) {
	defer s.wg.Done()

	server := &dns.Server{
		Net:         net,
		Addr:        addr,
		Handler:     s.mux,
		ReadTimeout: 1 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) allSOA() []dns.RR {
	records := []dns.RR{}
	for _, n := range s.navigators {
		records = append(records, n.NewSOA()...)
	}
	return records
}

func NewServer(config *Config) (*Server, error) {
	navigators := map[string]n.Navigator{}
	for _, c := range config.NavigatorConfigs {
		n, err := NewNavigator(c.Name, c.Domain, c.Config)
		if err != nil {
			return nil, fmt.Errorf("ERROR: invalid navigator %v with err: %v",
				c, err)
		}
		navigators[c.Domain] = n
	}
	if len(navigators) == 0 {
		return nil, fmt.Errorf("ERROR: empty navigators %v", config)
	}
	server := &Server{
		config:     config,
		wg:         new(sync.WaitGroup),
		navigators: navigators,
	}
	return server, nil
}
