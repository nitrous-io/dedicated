package containers

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

var (
	ErrSSHPortNotFound = errors.New("SSH Port not available in container")
	ErrSSHNotEnabled   = errors.New("SSH Port is not enabled")
	sshRegex           = regexp.MustCompile("^public:(\\d+):22(/tcp)?$")
)

func envMap(env []string) map[string]string {
	res := map[string]string{}
	for _, entry := range env {
		slices := strings.Split(entry, "=")
		if len(slices) == 2 {
			res[slices[0]] = slices[1]
		}
	}
	return res
}

func findSSHHostPort(ports []string) (string, error) {
	for _, port := range ports {
		matches := sshRegex.FindStringSubmatch(port)
		if matches != nil {
			return matches[1], nil
		}
	}
	return "", ErrSSHPortNotFound
}

func findSSHURL(h *host.Host, config *freighterclient.ContainerConfig) (string, error) {
	env := envMap(config.Env)

	enabled := env["NITROUS_SSH_ENABLED"]
	if enabled != "true" {
		return "", ErrSSHNotEnabled
	}

	user := env["NITROUS_USERNAME"]
	if user == "" {
		user = "root"
	}

	domain := h.Hostname()
	port, err := findSSHHostPort(config.PortBindings)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("ssh://%s@%s:%s", user, domain, port), nil
}
