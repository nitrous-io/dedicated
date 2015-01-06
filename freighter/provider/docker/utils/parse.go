package utils

import (
	"fmt"
	"strings"

	"github.com/docker/docker/nat"
	"github.com/fsouza/go-dockerclient"
)

var (
	ErrInvalidPortSpec = fmt.Errorf("port binding invalid")
)

// ParsePortBindings parses a list of freighterd-format port specs into docker
// port specs suitable to be used in docker configurations
func ParsePortBindings(specs []string, gateway string) (map[docker.Port][]docker.PortBinding, error) {
	portSpecs := make([]string, len(specs))
	for i, s := range specs {
		spec, err := parsePortSpec(s, gateway)
		if err != nil {
			return nil, err
		}
		portSpecs[i] = spec
	}

	// convert the specs into nat[Port][]Binding
	dockerBindings := make(map[docker.Port][]docker.PortBinding)
	_, natBindings, err := nat.ParsePortSpecs(portSpecs)
	if err != nil {
		return nil, err
	}

	// convert nat[Port][]Binding into go-dockerclient format
	for port, portBindings := range natBindings {
		currentBindings := make([]docker.PortBinding, len(portBindings))
		for i, b := range portBindings {
			currentBindings[i] = docker.PortBinding{b.HostIp, b.HostPort}
		}
		dockerBindings[docker.Port(port)] = currentBindings
	}

	return dockerBindings, err
}

// parsePortSpec supports any of:
//   public:hostPort:containerPort
//   private:hostPort:containerPort
//   public::containerPort
//   private::containerPort
//   hostPort:containerPort ... : (which defaults to private)
//
//   private means it will be bound to the gateway argument
//   public means that will be accessible on the internet
//
// it returns the format that can be parsed by docker
func parsePortSpec(portSpec, gateway string) (string, error) {
	parts := strings.Split(portSpec, ":")

	var hostIP string
	switch parts[0] {
	case "public":
		hostIP = "0.0.0.0"
	case "private":
		hostIP = gateway
	}
	switch {
	case len(parts) == 3 && parts[1] == "": // type::containerPort
		if hostIP == "" {
			return "", ErrInvalidPortSpec
		}
		return hostIP + "::" + parts[2], nil
	case len(parts) == 3 && parts[1] != "": // type:hostPort:containerPort
		if hostIP == "" {
			return "", ErrInvalidPortSpec
		}
		return hostIP + ":" + parts[1] + ":" + parts[2], nil
	case len(parts) == 2: // hostPort:containerPort
		return gateway + ":" + portSpec, nil
	}
	return "", ErrInvalidPortSpec
}
