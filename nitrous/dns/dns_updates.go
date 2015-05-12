package dns

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	nroute53 "github.com/nitrous-io/dedicated/nitrous/pkg/route53"
	"github.com/nitrous-io/nitrous-common/logger"
)

var (
	// list from https://github.com/nitrous-io/nitrous_cookbooks/blob/master/docs/deploy-production.md
	// maps provider_templates.config:region_id to the name of the server
	regionToTender = map[string]string{
		"us-east-1":      "amazon.nitrousproxy-sfo.com",
		"us-west-2":      "amazon.nitrousproxy-nyc.com",
		"eu-west-1":      "amazon.nitrousproxy-ams.com",
		"ap-southeast-1": "amazon.nitrousproxy-sgp.com",
	}
	errInvalidIPAddress = errors.New("invalid IPAddress")
	activeDNS           DNS
)

func init() {
	// nitrousd.go will update this if envvars are set accordingly
	SetActiveDNS(&MockDNS{})
}

type DNS interface {
	OnHostCreate(logger *logger.Logger, h *host.Host) error
	OnHostStart(logger *logger.Logger, h *host.Host) error
	OnHostDestroy(logger *logger.Logger, h *host.Host) error
	OnContainerCreate(logger *logger.Logger, cn *container.Container, h *host.Host) error
	OnContainerDestroy(logger *logger.Logger, cn *container.Container) error
}

type RealDNS struct {
}

func (d *RealDNS) OnHostCreate(logger *logger.Logger, h *host.Host) error {
	return onHostCreate(logger, h)
}

func (d *RealDNS) OnHostStart(logger *logger.Logger, h *host.Host) error {
	return onHostStart(logger, h)
}

func (d *RealDNS) OnHostDestroy(logger *logger.Logger, h *host.Host) error {
	return onHostDestroy(logger, h)
}

func (d *RealDNS) OnContainerCreate(logger *logger.Logger, cn *container.Container, h *host.Host) error {
	return onContainerCreate(logger, cn, h)
}

func (d *RealDNS) OnContainerDestroy(logger *logger.Logger, cn *container.Container) error {
	return onContainerDestroy(logger, cn)
}

func SetActiveDNS(active DNS) {
	activeDNS = active
}

// SetHostProxyCNAME adds or updates CNAME record for the host's proxy.
// Source CNAME is slug + global proxy host name, destination is name of
// tender proxy chosen based on provider_templates.config:region_id
func SetHostProxyCNAME(logger *logger.Logger, h *host.Host) error {
	proxyHost := strings.TrimPrefix(envvars.ProxyHostname, ".")
	from := h.Slug + "." + proxyHost
	regionID := h.ProviderTemplate.Config.Map["region_id"]
	tenderServer := regionToTender[regionID.String]
	if tenderServer == "" {
		return fmt.Errorf("no tender server for region '%s'", regionID.String)
	}
	if err := nroute53.SetCNAME(from, tenderServer); err != nil {
		logger.Errorf("nroute53.SetCNAME('%s', '%s') failed with '%s'\n", err, from, tenderServer)
		return err
	}
	return nil
}

// SetHostCNAME adds or updates CNAME record for the host
func SetHostCNAME(logger *logger.Logger, h *host.Host) error {
	if !h.IPAddress.Valid && h.IPAddress.String == "" {
		logger.Errorf("SetHostCNAME(): ip address invalid for host %d (%s)", h.ID, h.Slug)
		return errInvalidIPAddress
	}

	hostHostName := strings.TrimPrefix(envvars.HostHostname, ".")
	from := h.Slug + "." + hostHostName
	// desired name is: ec2-xx-xx-xx-xx.compute-1.amazonaws.com
	to := "ec2-" + h.IPAddress.String + ".compute-1.amazonaws.com"

	if err := nroute53.SetCNAME(from, to); err != nil {
		logger.Errorf("nroute53.SetCNAME() for host %d (%s) failed with '%s'", h.ID, h.Slug, err)
		return err
	}
	return nil
}

// onHostCreate performs dns updates needed after host has been created:
//  - set CNAME for proxy
//  - for hosts with static ips, add CNAME to ec2-xx-xx-xx-xx.compute-1.amazonaws.com
func onHostCreate(lg *logger.Logger, h *host.Host) error {
	err := SetHostProxyCNAME(lg, h)
	if err != nil {
		return err
	}
	return SetHostCNAME(lg, h)
}

// onHostStart performs dns updates needed when host is started:
// - for non-static-ip hosts, update the CNAME record for <hostslug>.HOST_HOSTNAME
func onHostStart(logger *logger.Logger, h *host.Host) error {
	if h.StaticIP {
		return nil
	}
	return SetHostCNAME(logger, h)
}

// onHostDestroy performs dns updates needed after host has been destroyed:
// - remove CNAME and A records
func onHostDestroy(logger *logger.Logger, h *host.Host) error {
	hostHostName := strings.TrimPrefix(envvars.HostHostname, ".")
	name := h.Slug + "." + hostHostName
	err := nroute53.DeleteExistingDNSRecords(name)
	if err != nil {
		logger.Errorf("nroute53.DeleteExistingDNSRecords('%s') failed with '%s'", name, err)
		return err
	}
	proxyHost := strings.TrimPrefix(envvars.ProxyHostname, ".")
	name = h.Slug + "." + proxyHost
	err = nroute53.DeleteExistingDNSRecords(name)
	if err != nil {
		logger.Errorf("nroute53.DeleteExistingDNSRecords('%s') failed with '%s'", name, err)
		return err
	}
	return nil
}

// onContainerCreate performs dns updates needed when container is created
// - add a CNAME record for <containerslug>.PREVIEW_HOSTNAME
func onContainerCreate(logger *logger.Logger, cn *container.Container, h *host.Host) error {
	from := cn.PreviewHostname()
	hostHostName := strings.TrimPrefix(envvars.HostHostname, ".")
	to := h.Slug + "." + hostHostName
	return nroute53.SetCNAME(from, to)
}

// onContainerDestroy performs dns updates needed when container is destroyed
// - remove CNAME record
func onContainerDestroy(logger *logger.Logger, cn *container.Container) error {
	return nroute53.DeleteExistingDNSRecords(cn.PreviewHostname())
}

func OnHostCreate(logger *logger.Logger, h *host.Host) error {
	return activeDNS.OnHostCreate(logger, h)
}

func OnHostStart(logger *logger.Logger, h *host.Host) error {
	return activeDNS.OnHostStart(logger, h)
}

func OnHostDestroy(logger *logger.Logger, h *host.Host) error {
	return activeDNS.OnHostDestroy(logger, h)
}

func OnContainerCreate(logger *logger.Logger, cn *container.Container, h *host.Host) error {
	return activeDNS.OnContainerCreate(logger, cn, h)
}

func OnContainerDestroy(logger *logger.Logger, cn *container.Container) error {
	return activeDNS.OnContainerDestroy(logger, cn)
}
