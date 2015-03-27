package compass

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

var testPort = 9999
var testData = map[string]string{
	"test.example.com.":  "127.0.0.1",
	"hello.example.com.": "127.0.0.1",
}

func testServer() (*Server, string, error) {
	addr := fmt.Sprintf("localhost:%d", testPort)
	config := &Config{
		Addr: fmt.Sprintf("localhost:%d", testPort),
		NavigatorConfigs: []NavigatorConfig{
			{
				Name:   "dummy",
				Domain: "example.com.",
				Config: testData,
			},
		},
	}
	s, _ := NewServer(config)
	go s.ListenAndServe()
	testPort += 1
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case <-timeout:
			return s, addr, errors.New("timed out starting server")
		default:
			_, err := net.DialTimeout("udp", addr, 10*time.Millisecond)
			if err == nil {
				return s, addr, nil
			}
			time.Sleep(time.Millisecond)
		}
	}
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

func assertA(t *testing.T, got dns.RR, expected dns.RR) {
	a := got.(*dns.A)
	b := expected.(*dns.A)
	if a.A.String() != b.A.String() ||
		a.Header().String() != b.Header().String() {
		t.Fatalf("Mismatched record. Got %v, expected %v", got, expected)
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
