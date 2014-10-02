package test

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/nitrous-io/dedicated/nitro/command"
)

func WriteConfig(conf *command.ConfigFile, path string) (string, error) {
	var f *os.File
	var err error
	if path == "" {
		f, err = ioutil.TempFile("", "nitrocfg")
	} else {
		f, err = os.Create(path)
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	os.Setenv("NITRO_CONFIG_PATH", f.Name())
	err = json.NewEncoder(f).Encode(conf)
	if err != nil {
		return "", err
	}
	command.Env = command.ConfigureEnvironment()
	return f.Name(), nil
}

// NewTLSServer returns a new, started httptest server with tls and the given cert
func NewTLSServer(handler http.Handler, hostname string, certPEM []byte) (*httptest.Server, error) {
	certPool, err := command.CACertPool()
	if err != nil {
		return nil, err
	}

	tlsCert, err := tls.X509KeyPair(SampleHostCert, SampleHostKey)
	if err != nil {
		return nil, err
	}

	svr := httptest.NewUnstartedServer(handler)
	svr.TLS = &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      certPool,
		ServerName:   hostname,
	}

	svr.StartTLS()
	return svr, nil
}

// signed with
// md5sum                            filename
// e8fc22617a15decf51825d905cb72838  dev/n2odevpro-ca.crt
// 73c8c5927399089dee63489357719067  dev/n2odevpro-ca.key
var (
	// keypair below generated using this hostname.
	// Generated using host cert generation test in
	// nitrous.
	SampleHostname = "test.n2odevpro.com"

	SampleHostCert = []byte(`-----BEGIN CERTIFICATE-----
MIIC1jCCAcCgAwIBAgIIY6qfG4FQwGcwCwYJKoZIhvcNAQELMGYxCzAJBgNVBAYT
AlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1TYW4gRnJhbmNpc2Nv
MRAwDgYDVQQKEwdOMk8gRGV2MRgwFgYDVQQDFA8qLm4yb2RldnByby5jb20wHhcN
MTQwOTA4MDk1MjAwWhcNMjQwOTA1MDk1MjAwWjCBgTELMAkGA1UEBhMCVVMxEjAQ
BgNVBAoTCU5pdHJvdXNJTzEUMBIGA1UECxMLRW5naW5lZXJpbmcxFjAUBgNVBAcT
DVNhbiBGcmFuY2lzY28xEzARBgNVBAgTCkNhbGlmb3JuaWExGzAZBgNVBAMTEnRl
c3QubjJvZGV2cHJvLmNvbTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQDi207VJdIw
6xvRQ6JtVkNnGq9hWeOFQRC0bPTmkMqlc2bsgoXMEQxb9+rp9XSMCF/zbmZDzwJc
dhYAA1vaK0qvAgMBAAGjODA2MBMGA1UdJQQMMAoGCCsGAQUFBwMBMB8GA1UdIwQY
MBaAFLnMum5YZ9Mdy43sc5oq2YqqHX4HMAsGCSqGSIb3DQEBCwOCAQEAXz/5Zv4W
JLqV5LQsG9DLB2mfSkGqHl+VUUNDWi9tPY1lPKjXkbb4bW5uYt506veO8WYpoxQM
owhu9wkaOUPSZLOBa+BIxqjHl78nF4KukpZYZGUpV1ytbg+J3YtCTX+aUpGv5K5k
7yavoipF0fAUMkq5L1BNRihg7Ttrk1Kr+kNa7XZkqzmNiRlGWoKhONEY3Bri09EC
HQB2AoIHUA4Eps++ejQadCg6P2xP+tRhFQIlEi5ig1/6iUkEn+WKDRMXF9m4anS2
/ylUYDifV8kZJ8efCjOUYjLj8XYXKFKGlo4lkYwSauolO78vQYCSsa48o8xhZoai
oSc8l3s4SEqL8g==
-----END CERTIFICATE-----`)

	SampleHostKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIBPAIBAAJBAOLbTtUl0jDrG9FDom1WQ2car2FZ44VBELRs9OaQyqVzZuyChcwR
DFv36un1dIwIX/NuZkPPAlx2FgADW9orSq8CAwEAAQJBAKAL0QYXdL8GX823RbSV
k1/+N5Ua8aVlTf+8jtrLc+xctcJdRGPGrWf0b+EGNwhhudYAf67ImhKSi9QYzvsl
vUECIQDj3Z3/Np8FkhYjgXRp682gXIUC6gZp++OK1NA9Y6wStwIhAP7dzDFa+1rj
prDmBecLBPDCBPlm+1RvHqfsYvZPZy/JAiAdtcqg+R0427hZPsrP+Lfh3c5V9++I
KBieBGt2ZaWM9wIhAKG6Nx++ny/Os/EfFfHF0U3BoIxup9+tX6OpYibwvgbZAiEA
x7fLa5Dg3FZ1igRqOy95AsyDkbbRzfDUQzY2q32IVNA=
-----END RSA PRIVATE KEY-----`)
)
