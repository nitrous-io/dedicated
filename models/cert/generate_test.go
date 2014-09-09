package cert_test

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/cert"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	cert.KeyLen = 512 // shorten key length so tests run faster
}

func startServer(certPool *x509.CertPool, hostCert tls.Certificate) *httptest.Server {
	svr := httptest.NewUnstartedServer(http.DefaultServeMux)
	svr.TLS = &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{hostCert},
		RootCAs:      certPool,
	}
	svr.StartTLS()
	return svr
}

var _ = Describe("models/cert", func() {
	Describe("GenerateHostCert", func() {
		It("generates a valid keypair", func() {
			hostname := "test.n2odevpro.com" // not hardcoded, can be anything
			caInfo, err := controllers.GetCAInfo(true)
			Expect(err).To(BeNil())

			certPool := x509.NewCertPool()
			certPool.AddCert(caInfo.X509Cert)

			certBytes, keyBytes, err := cert.GenerateHostCert(hostname, caInfo.X509Cert, caInfo.Key)
			hostCert, err := tls.X509KeyPair(certBytes, keyBytes)
			Expect(err).To(BeNil())

			svr := startServer(certPool, hostCert)
			defer svr.Close()

			client := &http.Client{}
			client.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					ServerName: hostname,
					RootCAs:    certPool,
				},
			}
			r, err := client.Get(svr.URL)
			Expect(err).To(BeNil())
			Expect(r.TLS.VerifiedChains).NotTo(BeEmpty())
			body, err := ioutil.ReadAll(r.Body)
			Expect(err).To(BeNil())
			Expect(string(body)).To(Equal("404 page not found\n"))
		})
	})
})
