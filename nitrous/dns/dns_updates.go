package dns

import (
	"errors"
	"fmt"

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
	OnHostCreate(h *host.Host) error
	OnHostStart(h *host.Host) error
	OnHostDestroy(h *host.Host) error
	OnContainerCreate(cn *container.Container) error
	OnContainerDestroy(cn *container.Container) error
}

type RealDNS struct {
	logger *logger.Logger
}

func (d *RealDNS) SetLogger(lg *logger.Logger) {
	d.logger = lg
}

type hostCNAMEs struct {
	proxyFrom, proxyTo       string
	hostnameFrom, hostnameTo string
}

func hostRecords(h *host.Host) (*hostCNAMEs, error) {
	regionID := h.ProviderTemplate.Config.Map["region_id"]
	proxyTo := regionToTender[regionID.String]
	if proxyTo == "" {
		return nil, fmt.Errorf("no tender server for region '%s'", regionID.String)
	}
	if !h.IPAddress.Valid || h.IPAddress.String == "" {
		return nil, errInvalidIPAddress
	}
	return &hostCNAMEs{
		proxyFrom:    fmt.Sprintf("%s.%s", h.Slug, envvars.ProxyHostname),
		proxyTo:      proxyTo,
		hostnameFrom: fmt.Sprintf("%s.%s", h.Slug, envvars.HostHostname),
		hostnameTo:   fmt.Sprintf("ec2-%s.compute-1.amazonaws.com", h.IPAddress.String),
	}, nil
}

type containerCNAMEs struct {
	previewFrom, previewTo string
}

func containerRecords(cn *container.Container) (*containerCNAMEs, error) {
	return &containerCNAMEs{
		previewFrom: cn.PreviewHostname(),
		previewTo:   fmt.Sprintf("%s.%s", cn.Slug, envvars.PreviewHostname),
	}, nil
}

// OnHostCreate performs dns updates needed after host has been created:
//  - set CNAME for proxy
//  - for hosts with static ips, add CNAME to ec2-xx-xx-xx-xx.compute-1.amazonaws.com
func (d *RealDNS) OnHostCreate(h *host.Host) error {
	err := d.setHostProxyCNAME(h)
	if err != nil {
		return err
	}
	return d.setHostCNAME(h)
}

// OnHostStart performs dns updates needed when host is started:
// - for non-static-ip hosts, update the CNAME record for <hostslug>.HOST_HOSTNAME
func (d *RealDNS) OnHostStart(h *host.Host) error {
	if h.StaticIP {
		return nil
	}
	return d.setHostCNAME(h)
}

// OnHostDestroy performs dns updates needed after host has been destroyed:
// - remove CNAME records
func (d *RealDNS) OnHostDestroy(h *host.Host) error {
	cnames, err := hostRecords(h)
	if err != nil {
		return err
	}
	if err := nroute53.DeleteCNAMERecord(cnames.hostnameFrom); err != nil {
		d.logger.Errorf("nroute53.DeleteCNAMERecord('%s') failed with '%s'", cnames.hostnameFrom, err)
		return err
	}
	if err = nroute53.DeleteCNAMERecord(cnames.proxyFrom); err != nil {
		d.logger.Errorf("nroute53.DeleteCNAMERecord('%s') failed with '%s'", cnames.proxyFrom, err)
		return err
	}
	return nil
}

// OnContainerCreate performs dns updates needed when container is created
// - add a CNAME record for <containerslug>.PREVIEW_HOSTNAME
func (d *RealDNS) OnContainerCreate(cn *container.Container) error {
	cnames, err := containerRecords(cn)
	if err != nil {
		return err
	}
	return nroute53.SetCNAME(cnames.previewFrom, cnames.previewTo)
}

// onContainerDestroy performs dns updates needed when container is destroyed
// - remove CNAME record
func (d *RealDNS) OnContainerDestroy(cn *container.Container) error {
	cnames, err := containerRecords(cn)
	if err != nil {
		return err
	}
	return nroute53.DeleteCNAMERecord(cnames.previewFrom)
}

func SetActiveDNS(active DNS) {
	activeDNS = active
}

// SetHostProxyCNAME adds or updates CNAME record for the host's proxy.
// Source CNAME is slug + global proxy host name, destination is name of
// tender proxy chosen based on provider_templates.config:region_id
func (d *RealDNS) setHostProxyCNAME(h *host.Host) error {
	cnames, err := hostRecords(h)
	if err != nil {
		return err
	}
	if err := nroute53.SetCNAME(cnames.proxyFrom, cnames.proxyTo); err != nil {
		d.logger.Errorf("nroute53.SetCNAME('%s', '%s') failed with '%s'\n", err, cnames.proxyFrom, cnames.proxyTo)
		return err
	}
	return nil
}

// SetHostCNAME adds or updates CNAME record for the host
func (d *RealDNS) setHostCNAME(h *host.Host) error {
	cnames, err := hostRecords(h)
	if err != nil {
		return err
	}
	if err := nroute53.SetCNAME(cnames.hostnameFrom, cnames.hostnameTo); err != nil {
		d.logger.Errorf("nroute53.SetCNAME() for host %d (%s) failed with '%s'", h.ID, h.Slug, err)
		return err
	}
	return nil
}

func OnHostCreate(h *host.Host) error {
	return activeDNS.OnHostCreate(h)
}

func OnHostStart(h *host.Host) error {
	return activeDNS.OnHostStart(h)
}

func OnHostDestroy(h *host.Host) error {
	return activeDNS.OnHostDestroy(h)
}

func OnContainerCreate(cn *container.Container, h *host.Host) error {
	return activeDNS.OnContainerCreate(cn)
}

func OnContainerDestroy(cn *container.Container) error {
	return activeDNS.OnContainerDestroy(cn)
}
