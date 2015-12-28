package compass

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/miekg/dns"
)

func init() { logrus.SetLevel(logrus.PanicLevel) }

var testPort = 9999
var testData = map[string]string{
	"test.example.com.":  "127.0.0.1",
	"hello.example.com.": "127.0.0.1",
}

func testServer() (*Server, string, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", testPort)
	config := &Config{
		Addr: addr,
		Nameservers: []string{
			"ns1.example.com.",
			"ns2.example.com.",
		},
		NavigatorConfigs: []NavigatorConfig{
			{
				Name:   "dummy",
				Domain: "example.com.",
				Config: testData,
			},
		},
	}
	s, err := NewServer(config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create server with config %v, err: %v", config, err)
	}
	go s.ListenAndServe()
	testPort++

	// Wait till server is started.
	ticker := time.NewTicker(500 * time.Millisecond)
	timeout := time.NewTimer(3 * time.Second)
	defer ticker.Stop()
	defer timeout.Stop()
	for {
		select {
		case <-ticker.C:
			if serverRunning(addr) {
				return s, addr, nil
			}

		case <-timeout.C:
			return s, addr, errors.New("timed out starting server")
		}
	}
}

func serverRunning(addr string) bool {
	conn, err := net.DialTimeout("udp", addr, 10*time.Millisecond)
	if err != nil {
		return false
	}

	// Send a test DNS message to verify that server is running.
	co := &dns.Conn{Conn: conn}
	defer co.Close()
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeNS)
	if err := co.WriteMsg(m); err != nil {
		return false
	}
	if _, err := co.ReadMsg(); err != nil {
		return false
	}

	return true
}

type dnsTestCase struct {
	qname  string
	qtype  uint16
	answer []dns.RR
}

func newA(rr string) *dns.A {
	r, _ := dns.NewRR(rr)
	return r.(*dns.A)
}

func newSOA(rr string) *dns.SOA {
	r, _ := dns.NewRR(rr)
	return r.(*dns.SOA)
}

func newNS(rr string) *dns.NS {
	r, _ := dns.NewRR(rr)
	return r.(*dns.NS)
}

func assertA(t *testing.T, got dns.RR, expected dns.RR) {
	a := got.(*dns.A)
	b := expected.(*dns.A)
	if a.A.String() != b.A.String() ||
		a.Header().String() != b.Header().String() {
		t.Fatalf("Mismatched A record. Got %v, expected %v", got, expected)
	}
}

func assertSOA(t *testing.T, got dns.RR, expected dns.RR) {
	a := got.(*dns.SOA)
	b := expected.(*dns.SOA)
	if a.Ns != b.Ns ||
		a.Mbox != b.Mbox ||
		a.Header().String() != b.Header().String() {
		t.Fatalf("Mismatched SOA record. Got %v, expected %v", got, expected)
	}
}

func assertNS(t *testing.T, got dns.RR, expected dns.RR) {
	a := got.(*dns.NS)
	b := expected.(*dns.NS)
	if a.String() != b.String() ||
		a.Header().String() != b.Header().String() {
		t.Fatalf("Mismatched NS record. Got %v, expected %v", got, expected)
	}
}

func assertAnswers(t *testing.T, got []dns.RR, expected []dns.RR) {
	if len(got) != len(expected) {
		t.Fatalf("Expected answers %v, got %v", expected, got)
	}
	for i, g := range got {
		switch g.(type) {
		case *dns.A:
			assertA(t, g, expected[i])
		case *dns.SOA:
			assertSOA(t, g, expected[i])
		case *dns.NS:
			assertNS(t, g, expected[i])
		default:
			t.Fatal("Unexpected answer type.")
		}
	}
}

func TestServeQuestions(t *testing.T) {
	s, addr, err := testServer()
	defer s.Stop()
	if err != nil {
		t.Fatal("failed to start server", err)
	}

	testCases := []dnsTestCase{
		{
			qname: "test.example.com.",
			qtype: dns.TypeA,
			answer: []dns.RR{newA("test.example.com. 300 A " +
				testData["test.example.com."])},
		},
		{
			qname:  "does-not-exist.example.com.",
			qtype:  dns.TypeA,
			answer: []dns.RR{},
		},
		{
			qname: "example.com.",
			qtype: dns.TypeSOA,
			answer: []dns.RR{newSOA(
				"example.com. 21600 IN SOA ns1.example.com. admin.example.com. 1430388000 28800 7200 604800 300",
			)},
		},
		{
			qname: "exampleproxy.com.",
			qtype: dns.TypeSOA,
			answer: []dns.RR{newSOA(
				"exampleproxy.com. 21600 IN SOA ns1.example.com. admin.exampleproxy.com. 1430388000 28800 7200 604800 300",
			)},
		},
		{
			qname: "example.com.",
			qtype: dns.TypeNS,
			answer: []dns.RR{
				newNS("example.com. 21600 IN NS ns1.example.com."),
				newNS("example.com. 21600 IN NS ns2.example.com."),
			},
		},
		{
			qname: "exampleproxy.com.",
			qtype: dns.TypeNS,
			answer: []dns.RR{
				newNS("exampleproxy.com. 21600 IN NS ns1.example.com."),
				newNS("exampleproxy.com. 21600 IN NS ns2.example.com."),
			},
		},
	}

	for _, tc := range testCases {
		c := new(dns.Client)
		m := new(dns.Msg)
		m.SetQuestion(tc.qname, tc.qtype)

		r, _, err := c.Exchange(m, addr)
		if err != nil {
			t.Fatal("Failed to get DNS response", err)
		}
		assertAnswers(t, r.Answer, tc.answer)
	}
}

func TestServeUnhandledDomain(t *testing.T) {
	s, addr, err := testServer()
	defer s.Stop()
	if err != nil {
		t.Fatal("failed to start server", err)
	}

	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion("does-not-exist.example.com.", dns.TypeA)
	r, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("Failed to get DNS response: %v", err)
	}
	if len(r.Answer) != 0 {
		t.Fatalf("Expected no answers but got %v", r.Answer)
	}
	if len(r.Ns) == 0 {
		t.Fatalf("Expected authority section to filled but it's empty")
	}
	soa, ok := r.Ns[0].(*dns.SOA)
	if !ok {
		t.Fatalf("Expected first NS record to be an SOA record but got %v", r.Ns[0])
	}
	if soa.Header().Name != "example.com." {
		t.Fatalf("Expected SOA header to have name example.com. but got %v", soa.Header().Name)
	}
	if r.Rcode != dns.RcodeNameError {
		t.Fatalf("Expected rcode to be NXDOMAIN but got %v", r.Rcode)
	}
}

func TestServeAAAA(t *testing.T) {
	s, addr, err := testServer()
	defer s.Stop()
	if err != nil {
		t.Fatal("failed to start server", err)
	}
	// AAAA -> NODATA for valid domain but we dont support AAAA
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeAAAA)
	r, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("failed to get dns response: %v", err)
	}
	if r.Rcode != dns.RcodeSuccess || len(r.Answer) > 0 {
		t.Errorf("expected NODATA but got %#v", r)
	}
	if len(r.Ns) == 0 {
		t.Errorf("expected authority section to filled but it's empty")
	}

	// AAAA -> NXDOMAIN for invalid domain
	m.SetQuestion("does-not-exist.example.com.", dns.TypeAAAA)
	r, _, err = c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("failed to get dns response: %v", err)
	}
	if r.Rcode != dns.RcodeNameError || len(r.Answer) > 0 {
		t.Errorf("expected NXDOMAIN but got %#v", r)
	}
	if len(r.Ns) == 0 {
		t.Fatalf("expected authority section to filled but it's empty")
	}
}

func TestNewServerWithEmptyNavigators(t *testing.T) {
	s, err := NewServer(&Config{
		Addr: fmt.Sprintf("localhost:%d", testPort),
	})
	if err == nil || s != nil {
		t.Fatal("Missing navigators should stop the world.")
	}
}

func TestNewServerWithInvalidNavigators(t *testing.T) {
	_, err := NewServer(&Config{
		Addr: fmt.Sprintf("localhost:%d", testPort),
		NavigatorConfigs: []NavigatorConfig{
			{"box", "example.com.", map[string]string{"database_url": ""}},
		},
	})
	if err == nil {
		t.Fatal("Invalid navigator should stop the world.")
	}
}
