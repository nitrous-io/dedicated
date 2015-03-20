package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/librato"
)

const defaultTTL = uint32(300)

var (
	ErrNotFound = errors.New("RecordNotFound")
)

// will be injected with the build process
var version string

type Navigator interface {
	Name() string // tell us what type of navigator you are.

	// Lookup returns DNS records for the given DNS name. Callers must ensure
	// that any Unicode characters outside of the ASCII set are converted to
	// Punycode.
	Lookup(name string) ([]dns.RR, error)

	NewSOA() []dns.RR
}

type NavigatorConfig struct {
	Name   string            `json:"name"`
	Domain string            `json:"domain"` // which domain this navigator will handle
	Config map[string]string `json:"config"` // configuration of the navigator
}

type Config struct {
	Addr             string            `json:"address"` // host:port
	LibratoUser      string            `json:"librato_user"`
	LibratoToken     string            `json:"librato_token"`
	NavigatorConfigs []NavigatorConfig `json:"navigators"`
}

type Server struct {
	config     *Config
	db         *sql.DB
	mux        *dns.ServeMux
	wg         *sync.WaitGroup
	navigators map[string]Navigator
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
		if len(m.Answer) > 1 {
			for _, r := range m.Answer {
				r.Header().Ttl = uint32(defaultTTL)
			}
		}
		w.WriteMsg(m)
	}()

	switch q.Qtype {
	case dns.TypeSOA:
		navigator, ok := s.navigators[strings.ToLower(q.Name)]
		if ok {
			m.Answer = navigator.NewSOA()
			return
		}
	case dns.TypeA:
		start := time.Now()
		for _, navigator := range s.navigators {
			label := fmt.Sprintf("compass.%s.lookup", navigator.Name())
			r, err := navigator.Lookup(q.Name)
			if err == nil {
				m.Answer = r
				mh.UpdateSince(label, start)
				log.Printf("Navigator %s: resolved %s to %+v", navigator.Name(), q.String(), r)
				return
			} else {
				mh.Error(err, label)
				log.Printf("Navigator %s: failed to resolve %s, err: %s", navigator.Name(), q.String(), err)
			}
		}
	}

	// if we cant handle the request, just return all our SOA as NS
	if len(m.Answer) == 0 {
		log.Printf("No answer for question %s", q.String())
		m.Ns = s.allSOA()
		m.Ns[0].Header().Ttl = defaultTTL
		m.Rcode = dns.RcodeNameError // NXDOMAIN
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

func NewNavigator(provider, domain string, config map[string]string) (Navigator, error) {
	switch provider {
	case "dummy":
		return NewDummyNavigator(domain, config)
	case "container":
		return NewContainerNavigator(domain, config)
	case "host":
		return NewHostNavigator(domain, config)
	default:
		return nil, fmt.Errorf("unknown provider '%s'", provider)
	}
}

func NewServer(config *Config) (*Server, error) {
	navigators := map[string]Navigator{}
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

func main() {
	flVersion := flag.Bool("v", false, "current version")
	flConfigPath := flag.String("c", "config.json", "config path")
	flag.Parse()
	if *flVersion {
		fmt.Printf("Compass: revision %s\n", version)
		os.Exit(0)
		return
	}
	configData, err := ioutil.ReadFile(*flConfigPath)
	if err != nil {
		fatal("Missing config file", *flConfigPath)
	}
	config := new(Config)
	err = json.Unmarshal(configData, &config)
	if err != nil {
		fatal("Invalid config file at ./config.json", err)
	}
	s, err := NewServer(config)
	if err != nil {
		fatal("Failed to start compass server:", err)
	}
	log.Println("compass: Name serving since 1978")
	if err := s.ListenAndServe(); err != nil {
		fatal("compass exited with error:", err)
	}
}

func fatal(a ...interface{}) {
	log.Println(a...)
	os.Exit(1)
}
