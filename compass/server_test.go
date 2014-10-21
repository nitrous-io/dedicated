package compass

import (
	"fmt"
	"testing"

	"github.com/miekg/dns"
)

var testPort = 9999
var testData = map[string]string{
	"test.example.com.":  "127.0.0.1",
	"hello.example.com.": "127.0.0.1",
}

func testServer() (*Server, string) {
	addr := fmt.Sprintf("localhost:%d", testPort)
	config := &Config{
		Addr: fmt.Sprintf("localhost:%d", testPort),
		NavigatorConfigs: []NavigatorConfig{
			{"dummy", "example.com.", testData},
		},
	}
	s, _ := NewServer(config)
	go s.ListenAndServe()
	testPort += 1
	return s, addr
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
		t.Fatal("Mismatched record. Got %v, expected %v", got, expected)
	}
}

func assertAnswers(t *testing.T, got []dns.RR, expected []dns.RR) {
	for i, g := range got {
		switch g.(type) {
		case *dns.A:
			assertA(t, g, expected[i])
		}
	}
}

func TestServeBoxhost(t *testing.T) {
	s, addr := testServer()
	defer s.Stop()

	testCases := []dnsTestCase{
		{
			qname: "test.n2oapp.com.",
			qtype: dns.TypeA,
			answer: []dns.RR{newA("test.example.com. 300 A " +
				testData["test.example.com."])},
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
