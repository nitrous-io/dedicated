package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"

	"golang.org/x/crypto/ssh"
)

const (
	caCertPath = "nitrous/dev/n2odevpro-ca.crt"
	caKeyPath  = "nitrous/dev/n2odevpro-ca.key"
	hostName   = ""
)

var (
	X509Cert *x509.Certificate
	Key      *rsa.PrivateKey
)

func fatalIfErr(err error) {
	if err != nil {
		log.Fatalf("error: %s\n", err)
	}
}

func loadCertMust(path string) *x509.Certificate {
	d, err := ioutil.ReadFile(path)
	fatalIfErr(err)
	block, _ := pem.Decode(d)
	fatalIfErr(err)
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	fatalIfErr(err)
	return x509Cert
}

func loadKeyMust(path string) *rsa.PrivateKey {
	d, err := ioutil.ReadFile(path)
	fatalIfErr(err)
	key, err := ssh.ParseRawPrivateKey(d)
	fatalIfErr(err)
	return key.(*rsa.PrivateKey)
}

func genCert() {
	//cert, err := cert.Generate(h.CertHostname(), X509Cert, Key)

}

func main() {
	fmt.Printf("")
}
