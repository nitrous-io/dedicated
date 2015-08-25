package main

import (
	"fmt"
	"os"
	"path"

	"github.com/Sirupsen/logrus"

	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostkeys"
)

const (
	privKeyFmt = "userhost_%d_%s.pem" // format with id, slug
)

type AnsibleHost struct {
	*hostkeys.SshHost
}

func inventoryHosts() ([]*AnsibleHost, error) {
	hosts := []*AnsibleHost{}
	err := dbConn.Select(&hosts, hostkeys.Query(
		`LEFT JOIN subscriptions AS subs ON hosts.subscription_id = subs.id
		WHERE hosts.terminated_at IS NULL
		AND hosts.state NOT IN ($1, $2)
		AND COALESCE(subs.state, 'null') != $3
		ORDER BY id;`),
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

	pem, err := h.PrivatePEM()
	if err != nil {
		return err
	}
	if _, err := f.Write(pem); err != nil {
		return err
	}
	return nil
}
