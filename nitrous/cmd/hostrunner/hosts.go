package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path"

	"github.com/Sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/nitrous-common/cipher"
)

const (
	privKeyFmt = "userhost_%d_%s.pem" // format with id, slug
)

type AnsibleHost struct {
	*host.Host
}

func inventoryHosts() ([]*AnsibleHost, error) {
	hosts := []*AnsibleHost{}
	err := dbConn.Select(&hosts, `SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug, orgs.slug AS org_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			LEFT JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
			LEFT JOIN orgs ON hosts.org_id = orgs.id
			WHERE terminated_at IS NULL
				AND pt.name NOT LIKE '%hotswap%'
				AND preprovisioned = FALSE
				AND hosts.state NOT IN ($1, $2)
				AND (
					sub.state IS NULL OR -- users with no subscriptions still exist
					sub.state != $3 -- deleted subscriptions will be deleted shortly
				)
			ORDER BY id;`,
		host.StateOrphan, host.StateBorn, subscription.StateDeleted)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func (h *AnsibleHost) FullKeyPath() string {
	return path.Join(workDir, h.keyPath())
}

func (h *AnsibleHost) keyPath() string {
	return fmt.Sprintf(privKeyFmt, h.ID, h.Slug)
}

func (h *AnsibleHost) writePrivateKey() error {
	p := h.FullKeyPath()
	if _, err := os.Stat(p); err == nil {
		// file exists, skip
		return nil
	}

	logrus.Debugf("Writing to %s", p)

	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	privKey, err := privateKey(h.Host)
	if err != nil {
		return err
	}

	if _, err := f.Write(privatePEM(privKey)); err != nil {
		return err
	}
	return nil
}

func privatePEM(priv *rsa.PrivateKey) []byte {
	// ASN.1 DER encoded form
	privder := x509.MarshalPKCS1PrivateKey(priv)
	privblk := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privder,
	}
	return pem.EncodeToMemory(&privblk)
}

func privateKey(h *host.Host) (*rsa.PrivateKey, error) {
	kp, err := keypairs.Get(dbConn, h.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find a keypair for host %v, error %q", h, err)
	}

	// get the private key from keypair
	priv, err := cipher.Decrypt(
		[]byte(aesKey),
		[]byte(kp.EncryptedPrivateKeyIv),
		[]byte(kp.EncryptedPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key for host %v, error %q", h, err)
	}

	pk, err := ssh.ParseRawPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key for host %v, error %q", h, err)
	}
	privKey, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid RSA key for host %v", h)
	}

	return privKey, nil
}
