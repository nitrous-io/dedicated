package cert_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	cert.KeyLen = 512 // shorten key length so tests run faster
}

var _ = Describe("models/cert", func() {
	Describe("GenerateHostCert", func() {
		It("generates a valid keypair", func() {
			tc := newTestContext("test.n2odevpro.com")
			defer tc.svr.Close()

			client := tc.newClient(tc.hostname)
			r, err := client.Get(tc.svr.URL)
			Expect(err).To(BeNil())
			Expect(r.TLS.VerifiedChains).NotTo(BeEmpty())
			body, err := ioutil.ReadAll(r.Body)
			Expect(err).To(BeNil())
			Expect(string(body)).To(Equal("404 page not found\n"))
		})

		It("does not connect with different hostname", func() {
			tc := newTestContext("test.n2odevpro.com")
			defer tc.svr.Close()

			client := tc.newClient("different.example.com")
			_, err := client.Get(tc.svr.URL)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("valid for %s, not different.example.com", tc.hostname)))
		})

		It("does not connect with host root CA set", func() {
			tc := newTestContext("test.n2odevpro.com")
			defer tc.svr.Close()

			tc.certPool = nil // nil RootCAs defaults to host root CAs
			client := tc.newClient(tc.hostname)
			_, err := client.Get(tc.svr.URL)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("signed by unknown authority"))
		})
	})
})

type testContext struct {
	hostname string
	svr      *httptest.Server
	certPool *x509.CertPool
}

func newTestContext(hostname string) *testContext {
	tc := &testContext{hostname: hostname}

	caInfo, err := certauth.NewCAInfo(appconfig.CACertPath, appconfig.CAKeyPath)
	Expect(err).To(BeNil())

	tc.certPool = x509.NewCertPool()
	tc.certPool.AddCert(caInfo.X509Cert)

	certBytes, keyBytes, err := cert.GenerateHostCert(tc.hostname, caInfo.X509Cert, caInfo.Key)
	hostCert, err := tls.X509KeyPair(certBytes, keyBytes)
	Expect(err).To(BeNil())
	for _, certData := range hostCert.Certificate {
		cert, err := x509.ParseCertificate(certData)
		Expect(err).To(BeNil())
		Expect(cert.PublicKeyAlgorithm).To(Equal(x509.RSA))
		Expect(cert.SignatureAlgorithm).To(Equal(x509.SHA256WithRSA))
	}

	tc.startServer(hostCert)
	return tc
}

func (tc *testContext) startServer(hostCert tls.Certificate) {
	tc.svr = httptest.NewUnstartedServer(http.DefaultServeMux)
	tc.svr.TLS = &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{hostCert},
		RootCAs:      tc.certPool,
	}
	// squelch TLS error logs
	tc.svr.Config.ErrorLog = log.New(ioutil.Discard, "", 0)
	tc.svr.StartTLS()
}

func (tc *testContext) newClient(hostname string) *http.Client {
	tlsConfig := &tls.Config{
		ServerName: hostname,
		RootCAs:    tc.certPool,
	}
	client := &http.Client{}
	client.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	return client
}
