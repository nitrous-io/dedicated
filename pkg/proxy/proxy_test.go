package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() { logrus.SetLevel(logrus.PanicLevel) }

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "proxy")
}

var _ = Describe("ProxyHTTP", func() {
	It("proxies HTTP request", func() {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello world"))
		}))
		defer s.Close()
		addr := s.Listener.Addr().String()
		hf := ProxyHTTP(addr, nil, nil)

		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "/", nil)
		hf(w, r)
		Expect(w.Body.String()).To(Equal("hello world"), "unexpected response")
	})

	It("proxies HTTP request with SSL", func() {
		s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello world"))
		}))
		defer s.Close()
		addr := s.Listener.Addr().String()
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(localhostCert)
		hf := ProxyHTTP(addr, &tls.Config{
			ServerName: "example.com",
			RootCAs:    certPool,
		}, nil)

		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "/", nil)
		hf(w, r)
		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Body.String()).To(Equal("hello world"), "unexpected response")
	})
})

var _ = Describe("ProxyWebsocket", func() {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	It("proxies websocket request", func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ws.WriteMessage(websocket.TextMessage, []byte("hello world"))
			ws.Close()
		}))
		defer ts.Close()
		addr := ts.Listener.Addr().String()

		s := httptest.NewServer(ProxyWebsocket(addr, nil))
		defer s.Close()

		c, _, err := websocket.DefaultDialer.Dial(strings.Replace(s.URL, "http", "ws", 1), nil)
		Expect(err).To(BeNil(), "failed to get a websocket connection")

		msgType, msg, err := c.ReadMessage()
		Expect(err).To(BeNil(), "failed to read the msg from websocket")
		Expect(msgType).To(Equal(websocket.TextMessage), "invalid msg type")
		Expect(string(msg)).To(Equal("hello world"), "unexpected response")
	})

	It("proxies websocket request with SSL", func() {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ws.WriteMessage(websocket.TextMessage, []byte("hello world"))
			ws.Close()
		}))
		defer ts.Close()
		addr := ts.Listener.Addr().String()

		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(localhostCert)
		s := httptest.NewServer(ProxyWebsocket(addr, &tls.Config{
			ServerName: "example.com",
			RootCAs:    certPool,
		}))
		defer s.Close()

		c, _, err := websocket.DefaultDialer.Dial(strings.Replace(s.URL, "http", "ws", 1), nil)
		Expect(err).To(BeNil(), "failed to get a websocket connection")

		msgType, msg, err := c.ReadMessage()
		Expect(err).To(BeNil(), "failed to read the msg from websocket")
		Expect(msgType).To(Equal(websocket.TextMessage), "invalid msg type")
		Expect(string(msg)).To(Equal("hello world"), "unexpected response")
	})
})

var _ = Describe("IsWebsocket", func() {
	It("should return false if request header Upgrade is not set to websocket", func() {
		r, err := http.NewRequest("GET", "ws://example.com", nil)
		Expect(err).To(BeNil())
		r.Header.Set("Upgrade", "flash")
		r.Header.Set("Connection", "upgrade")
		Expect(IsWebsocket(r)).To(BeFalse())
	})

	It("should return false if request header Connection does not contain upgrade", func() {
		r, err := http.NewRequest("GET", "ws://example.com", nil)
		Expect(err).To(BeNil())
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Connection", "lolupgrade")
		Expect(IsWebsocket(r)).To(BeFalse())
	})

	It("should return true if request header Connection contains only upgrade", func() {
		r, err := http.NewRequest("GET", "ws://example.com", nil)
		Expect(err).To(BeNil())
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Connection", "upgrade")
		Expect(IsWebsocket(r)).To(BeTrue())
	})

	It("should return true if request header Connection contains upgrade and other values", func() {
		r, err := http.NewRequest("GET", "ws://example.com", nil)
		Expect(err).To(BeNil())
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Connection", "Upgrade, keep-alive")
		Expect(IsWebsocket(r)).To(BeTrue())
	})
})

var _ = Describe("headerContainsValue", func() {
	It("returns true or false depending on whether header contains the given value", func() {
		testCases := []struct {
			value    string
			expected bool
		}{
			{"Upgrade", true},
			{"upgrade", true},
			{"UPGRADE", true},
			{"keep-alive, upgrade", true},
			{"keep-alive,upgrade", true},
			{"keep-alive,Upgrade", true},
			{"upgrades", false},
			{"keep-alive upgrade", false},
		}

		for _, tc := range testCases {
			h := http.Header{"Connection": {tc.value}}
			ok := headerContainsValue(h, "Connection", "upgrade")
			Expect(ok).To(Equal(tc.expected), "Expected headerContainsValue(h, n, %q) = %v, but got %v", tc.value, tc.expected, ok)
		}
	})
})

// localhostCert is a PEM-encoded TLS cert with SAN IPs
// "127.0.0.1" and "[::1]", expiring at Jan 29 16:00:00 2084 GMT.
// generated from src/crypto/tls:
// go run generate_cert.go  --rsa-bits 1024 --host 127.0.0.1,::1,example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var localhostCert = []byte(`-----BEGIN CERTIFICATE-----
MIICEzCCAXygAwIBAgIQMIMChMLGrR+QvmQvpwAU6zANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIGfMA0GCSqGSIb3DQEBAQUAA4GNADCB
iQKBgQDuLnQAI3mDgey3VBzWnB2L39JUU4txjeVE6myuDqkM/uGlfjb9SjY1bIw4
iA5sBBZzHi3z0h1YV8QPuxEbi4nW91IJm2gsvvZhIrCHS3l6afab4pZBl2+XsDul
rKBxKKtD1rGxlG4LjncdabFn9gvLZad2bSysqz/qTAUStTvqJQIDAQABo2gwZjAO
BgNVHQ8BAf8EBAMCAqQwEwYDVR0lBAwwCgYIKwYBBQUHAwEwDwYDVR0TAQH/BAUw
AwEB/zAuBgNVHREEJzAlggtleGFtcGxlLmNvbYcEfwAAAYcQAAAAAAAAAAAAAAAA
AAAAATANBgkqhkiG9w0BAQsFAAOBgQCEcetwO59EWk7WiJsG4x8SY+UIAA+flUI9
tyC4lNhbcF2Idq9greZwbYCqTTTr2XiRNSMLCOjKyI7ukPoPjo16ocHj+P3vZGfs
h1fIw3cSS2OolhloGw/XM6RWPWtPAlGykKLciQrBru5NAPvCMsb/I1DAceTiotQM
fblo6RBxUQ==
-----END CERTIFICATE-----`)
