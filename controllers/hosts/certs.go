package hosts

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/cert"
)

var (
	// how long till certs expire
	defaultCertDuration = 10 * 365 * 24 * time.Hour

	KeyLen = 2048
)

// generate cert with server certificate and key. The cert will be used for
// TLS encryption
func generateCerts(hostname string) (*cert.Cert, error) {
	caInfo, err := controllers.GetCAInfo(true)
	if err != nil {
		return nil, err
	}
	srvCrt, srvKey, err := GenerateHostCert(hostname, caInfo.X509Cert, caInfo.Key)
	if err != nil {
		return nil, err
	}

	return &cert.Cert{
		ServerCert: string(srvCrt),
		ServerKey:  string(srvKey),
	}, nil
}

// GenerateHostCert takes a hostname and root CA keypair and returns a keypair
// signed and issued by the CA
func GenerateHostCert(
	hostname string,
	caCert *x509.Certificate,
	caKey *rsa.PrivateKey,
) (hostCert []byte, hostKey []byte, err error) {
	subject := pkix.Name{
		Country:            []string{"US"},
		Province:           []string{"California"},
		Locality:           []string{"San Francisco"},
		Organization:       []string{"NitrousIO"},
		OrganizationalUnit: []string{"Engineering"},
		CommonName:         hostname,
	}

	privKey, err := rsa.GenerateKey(rand.Reader, KeyLen)
	if err != nil {
		return nil, nil, err
	}

	creq, err := hostCertReq(subject, privKey)
	if err != nil {
		return nil, nil, err
	}

	serial, err := genSerial()
	if err != nil {
		return nil, nil, err
	}

	certTemp := &x509.Certificate{
		SerialNumber: serial,
		Issuer:       caCert.Subject,
		Subject:      creq.Subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(defaultCertDuration),

		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		SignatureAlgorithm: creq.SignatureAlgorithm,
		DNSNames:           creq.DNSNames,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, certTemp, caCert, &privKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	return encodeCert(certBytes), encodeKey(privKey), nil
}

// hostCertReq populates a certificate request for a host cert, signed with
// host's privKey
func hostCertReq(subject pkix.Name, privKey *rsa.PrivateKey) (*x509.CertificateRequest, error) {
	template := &x509.CertificateRequest{Subject: subject}
	csr, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificateRequest(csr)
}

func encodeKey(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func encodeCert(cert []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})
}

func genSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 64)
	return rand.Int(rand.Reader, limit)
}
