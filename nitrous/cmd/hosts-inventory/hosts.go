package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

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

func inventoryHostsFromFile(f *os.File) ([]*AnsibleHost, error) {
	// TODO: couldn't get sqlx.In working with the needed query, sacrifice some
	// performance for now
	hosts, err := inventoryHosts()
	if err != nil {
		return nil, err
	}

	slugs := map[string]struct{}{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, "#") {
			continue
		}
		slugs[t] = struct{}{}
	}

	filtered := []*AnsibleHost{}
	for _, h := range hosts {
		if _, ok := slugs[h.Slug]; !ok {
			continue
		}
		filtered = append(filtered, h)
	}

	return filtered, nil
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
