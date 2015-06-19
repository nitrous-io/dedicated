package billingprofile

import (
	"bufio"
	"fmt"
	"net"
	"os"

	"github.com/Sirupsen/logrus"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
)

var (
	CIDRBlacklist []*net.IPNet
)

func init() {
	var err error
	CIDRBlacklist, err = importBlacklist()
	if err != nil {
		logrus.Warnf("Failed to initalize CIDR blacklist: %v", err)
	}
}

func importBlacklist() ([]*net.IPNet, error) {
	f, err := os.Open(envvars.CIDRBlacklistPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	networks := []*net.IPNet{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		line := scanner.Text()
		if line[0] == '#' {
			continue
		}
		_, n, err := net.ParseCIDR(line)
		if err != nil {
			return nil, err
		}
		networks = append(networks, n)
	}
	return networks, nil
}

func CIDRBlacklisted(ip string) error {
	source := net.ParseIP(ip)
	for _, n := range CIDRBlacklist {
		if ip := source.Mask(n.Mask); n.Contains(ip) {
			return fmt.Errorf("blocked by CIDR %s", n.String())
		}
	}
	return nil
}
