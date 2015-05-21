package compass

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	n "github.com/nitrous-io/dedicated/compass/navigator"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/librato"
)

const (
	defaultDomainTTL = uint32(300)
	defaultSOATTL    = uint32(21600)
	defaultNSTTL     = uint32(21600)
)

type NavigatorConfig struct {
	Name   string            `json:"name"`
	Domain string            `json:"domain"` // which domain this navigator will handle
	Config map[string]string `json:"config"` // configuration of the navigator
}

type Config struct {
	Addr             string            `json:"address"` // host:port
	Nameservers      []string          `json:"nameservers"`
	LibratoUser      string            `json:"librato_user"`
	LibratoToken     string            `json:"librato_token"`
	NavigatorConfigs []NavigatorConfig `json:"navigators"`
}

type Server struct {
	config     *Config
	db         *sql.DB
	mux        *dns.ServeMux
	wg         *sync.WaitGroup
	navigators map[string]n.Navigator
}

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
	log.Printf("Question: %s", q.String())

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer = make([]dns.RR, 0, 10)

	// always write dns message back
	defer func() {
		w.WriteMsg(m)
	}()

	switch q.Qtype {
	case dns.TypeSOA:
		m.Answer = s.soa(q.Name)
		log.Printf("Answering SOA query with %#v", m)
		return
	case dns.TypeNS:
		m.Answer = s.ns(q.Name)
		log.Printf("Answering NS query with %#v", m)
		return
	case dns.TypeA, dns.TypeAAAA:
		start := time.Now()
		navigator, ok := s.navigatorForA(q)
		if !ok {
			s.setNxdomain(q, m)
			return
		}
		label := fmt.Sprintf("compass.%s.lookup", navigator.Name())
		res, err := navigator.Lookup(q.Name)
		if err != nil {
			mh.Error(err, label)
			log.Printf("Navigator %s: failed to resolve %s, err: %s", navigator.Name(), q.String(), err)
			s.setNxdomain(q, m)
			return
		}
		mh.UpdateSince(label, start)

		// if it's TypeAAAA, returns NODATA
		if q.Qtype == dns.TypeAAAA {
			m.Rcode = dns.RcodeSuccess
			m.Ns = s.soa(q.Name)
			log.Printf("Navigator %s: resolved %s to NODATA", navigator.Name(), q.String())
		} else {
			m.Answer = res
			log.Printf("Navigator %s: resolved %s to %#v", navigator.Name(), q.String(), r)
		}
		if len(m.Answer) > 1 {
			for _, r := range m.Answer {
				r.Header().Ttl = uint32(defaultDomainTTL)
			}
		}
		return
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

func (s *Server) navigatorForA(qn dns.Question) (n.Navigator, bool) {
	rootDomain := n.RootDomain(strings.ToLower(qn.Name))
	n, ok := s.navigators[rootDomain]
	return n, ok
}

func (s *Server) setNxdomain(q dns.Question, m *dns.Msg) {
	log.Printf("No answer for question %s", q.String())
	m.Rcode = dns.RcodeNameError // NXDOMAIN
	m.Ns = s.allSOA()
}

func (s *Server) soa(name string) []dns.RR {
	if len(s.config.Nameservers) < 1 {
		return nil
	}
	primaryNS := s.config.Nameservers[0]
	return []dns.RR{
		&dns.SOA{
			Hdr:     dns.RR_Header{Name: name, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: defaultSOATTL},
			Ns:      primaryNS,
			Mbox:    "admin." + strings.ToLower(name),
			Serial:  uint32(time.Now().Truncate(time.Hour).Unix()),
			Refresh: 28800,
			Retry:   7200,
			Expire:  604800,
			Minttl:  defaultSOATTL,
		},
	}
}

func (s *Server) ns(name string) []dns.RR {
	rr := []dns.RR{}
	for _, ns := range s.config.Nameservers {
		rr = append(rr, &dns.NS{
			Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: defaultNSTTL},
			Ns:  ns,
		})
	}
	return rr
}

func (s *Server) allSOA() []dns.RR {
	rr := []dns.RR{}
	for _, c := range s.config.NavigatorConfigs {
		rr = append(rr, s.soa(c.Domain)...)
	}
	return rr
}

func NewServer(config *Config) (*Server, error) {
	navigators := map[string]n.Navigator{}
	for _, c := range config.NavigatorConfigs {
		if n, ok := navigators[c.Domain]; ok {
			return nil, fmt.Errorf("ERROR: cannot register navigator %s for domain %s because navigator %s is already registered", c.Name, c.Domain, n.Name())
		}

		n, err := NewNavigator(c.Name, c.Domain, c.Config)
		if err != nil {
			return nil, fmt.Errorf("ERROR: invalid navigator %v with err: %v",
				c, err)
		}
		navigators[c.Domain] = n
		log.Printf("Registered navigator %s for domain %s", n.Name(), c.Domain)
	}
	if len(navigators) == 0 {
		return nil, fmt.Errorf("ERROR: empty navigators %v", config)
	}
	server := &Server{
		config:     config,
		wg:         new(sync.WaitGroup),
		navigators: navigators,
	}

	// configure metrics output. if we do not have librato user/token i.e.
	// development environment, output the metrics to stderr
	// by default we flush metrics every 10s
	if config.LibratoUser == "" || config.LibratoToken == "" {
		log.Printf("missing librato config. Metrics logging is disabled")
		// in dev, we flush metrics every 60s into devNull
		// because we are not interested in them
		go metrics.Log(metrics.DefaultRegistry, 60e9,
			log.New(ioutil.Discard, "compass: metrics: ", log.Lmicroseconds))
	} else {
		go librato.Librato(metrics.DefaultRegistry,
			30e9,                // interval
			config.LibratoUser,  // account owner email address
			config.LibratoToken, // Librato API token
			"compass",           // source
			[]float64{0.99},     // precentiles to send
			time.Millisecond,    // time unit
		)
	}

	return server, nil
}
