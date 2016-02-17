package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/docker/nat"
	docker "github.com/fsouza/go-dockerclient"
)

var (
	ErrInvalidPortSpec = fmt.Errorf("port binding invalid")
)

// ParseRepoAndTag extracts the repo name and tag from a repository string.
// Examples:
//    ubuntu -> "ubuntu" + ""
//    ubuntu:14.04 -> "ubuntu" + "14.04"
//    nitrousio/ubuntu:14.04 -> "nitrousio/ubuntu" + "14.04"
//    example.com:5000/ubuntu:14.04 -> "example.com:5000/ubuntu" + "14.04"
//    example.com:5000/nitrousio/ubuntu:14.04 -> "example.com:5000/nitrousio/ubuntu" + "14.04"
// Stolen from https://github.com/docker/docker/blob/403df1765abfd3ac62b7f6b8bd709fb0d691d8c8/pkg/parsers/parsers.go#L72.
func ParseRepoAndTag(repos string) (string, string) {
	n := strings.LastIndex(repos, ":")
	if n < 0 {
		return repos, ""
	}
	tag := repos[n+1:]
	if !strings.Contains(tag, "/") {
		return repos[:n], tag
	}
	return repos, ""
}

var hostnameRe = regexp.MustCompile(`\A[^.]+\.[^.:]+(:[0-9]+)?\z`)

// ParseRegistry extracts the registry host (in "hostname:port" format).
// Examples:
//    nitrousio/ubuntu:14.04 -> ""
//    example.com/ubuntu:14.04 -> "example.com"
//    example.com:5000/ubuntu:14.04 -> "example.com:5000"
//    example.com:5000/nitrousio/ubuntu:14.04 -> "example.com:5000"
var ParseRegistry = func(repos string) string {
	parts := strings.Split(repos, "/")
	switch len(parts) {
	case 2:
		if hostnameRe.Match([]byte(parts[0])) {
			return parts[0]
		}
	case 3:
		return parts[0]
	default:
	}
	return ""
}

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

	switch {
	case len(parts) == 3: // type:hostPort:containerPort
		hostIP := parts[0]
		switch parts[0] {
		case "public":
			hostIP = "0.0.0.0"
		case "private":
			hostIP = gateway
		}
		if hostIP == "" {
			return "", ErrInvalidPortSpec
		}
		return hostIP + ":" + parts[1] + ":" + parts[2], nil
	case len(parts) == 2: // hostPort:containerPort
		return gateway + ":" + portSpec, nil
	}
	return "", ErrInvalidPortSpec
}

var (
	// w IDLE column is > 1 minute, < 1 hour
	wMinsRe = regexp.MustCompile(`\A(\d+):(\d\d)\z`) // MM:SS or M:SS
	// w IDLE column is < 1 minute
	wSecsRe = regexp.MustCompile(`\A(\d+)\.\d\ds\z`) // SS.CCs or S.CCs
)

// parse WOutput returns true if the command indicates that the user was active
// (their idle time is below the threshold).  Upon idle or unexpected input,
// returns false.
func ParseWOutput(output, username string, threshold int) (bool, error) {
	// output from `w --no-header <user>` (4th column is idle time) looks like:
	// nitrous  pts/1                     03:11    2:25m  0.08s  0.08s -zsh
	//
	// IDLE format:
	// The standard format is DDdays, HH:MMm, [M]M:SS or [S]S.CCs if the times are
	// greater than 2 days, 1hour, or 1 minute respectively.

	if len(output) == 0 {
		return false, nil
	}
	if threshold >= 60*60 {
		// thresholds an hour or more are not supported -- it is unlikely to be
		// a requirement, and it saves code
		return false, fmt.Errorf("threshold %d must be less than 3600", threshold)
	}
	lines := strings.Split(output, "\n")
	// if any line is under the threshold, that indicates an active console
	// session, so we return true
	for _, line := range lines {
		parts := strings.Fields(line)
		// This can happen when last character is `\n`
		if len(parts) == 0 {
			continue
		}

		if len(parts) < 7 {
			return false, fmt.Errorf("line did not match expected format: %s", line)
		}
		if parts[0] != username {
			return false, fmt.Errorf("line first column %s did not match username %s", parts[0], username)
		}
		idleStr := parts[3]
		minsIdle := wMinsRe.FindStringSubmatch(idleStr)
		secsIdle := wSecsRe.FindStringSubmatch(idleStr)
		switch {
		case strings.HasSuffix(idleStr, "days"), strings.HasSuffix(idleStr, "m"):
			continue
		case len(minsIdle) == 3:
			mins, err := strconv.Atoi(minsIdle[1])
			if err != nil {
				return false, err
			}
			secs, err := strconv.Atoi(minsIdle[2])
			if err != nil {
				return false, err
			}
			if 60*mins+secs < threshold {
				return true, nil
			}
		case len(secsIdle) == 2:
			secs, err := strconv.Atoi(secsIdle[1])
			if err != nil {
				return false, err
			}
			if secs < threshold {
				return true, nil
			}
		default:
			return false, fmt.Errorf("unexpected idle string %s", idleStr)
		}
	}

	return false, nil
}
