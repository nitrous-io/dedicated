package dns

import (
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	nroute53 "github.com/nitrous-io/dedicated/nitrous/pkg/route53"
	"github.com/nitrous-io/nitrous-common/logger"
)

var activeDNS DNS

func init() {
	// nitrousd.go will update this if envvars are set accordingly
	SetActiveDNS(&MockDNS{})
}

type DNS interface {
	OnHostCreate(h *host.Host) error
	OnHostStart(h *host.Host) error
	OnHostDestroy(h *host.Host) error
	OnContainerCreate(cn *container.Container, h *host.Host) error
	OnContainerDestroy(cn *container.Container) error
}

type RealDNS struct {
	logger *logger.Logger
}

func (d *RealDNS) SetLogger(lg *logger.Logger) {
	d.logger = lg
}

// OnHostCreate creates DNS records needed after host has been created:
// 1. CNAME <host-slug>.PROXY_HOSTNAME to Tender server for the host's region.
//    e.g. CNAME my-workspace.nitrousproxy.com to amazon.nitrousproxy-sgp.com
// 2. CNAME <host-slug>.HOST_HOSTNAME to EC2 instance public DNS.
//    e.g. CNAME my-workspace.nitrouspro.com to ec2-11-22-33-44.compute-1.amazonaws.com
func (d *RealDNS) OnHostCreate(h *host.Host) error {
	if err := d.setHostProxyCNAME(h); err != nil {
		return err
	}
	return d.setHostCNAME(h)
}

// OnHostStart updates DNS records needed when host is started:
// 1. If host does not have a static IP, CNAME <host-slug>.HOST_HOSTNAME to ec2-xx-xx-xx-xx.compute-1.amazonaws.com.
//    e.g. CNAME my-workspace.nitrouspro.com to ec2-11-22-33-44.compute-1.amazonaws.com
// 2. If host has a static IP, do nothing because we've already setup the CNAME
//    on create and the host's public DNS will not change.
func (d *RealDNS) OnHostStart(h *host.Host) error {
	if h.StaticIP {
		return nil
	}
	return d.setHostCNAME(h)
}

// OnHostDestroy deletes DNS records after host has been destroyed:
// 1. Remove CNAME to Tender server.
// 2. Remove CNAME to EC2 instance public DNS.
// 3. Remove DNS records of its containers.
func (d *RealDNS) OnHostDestroy(h *host.Host) error {
	records := []string{
		h.Hostname(),
		h.ProxyHostname(),
	}

	for _, r := range records {
		if err := nroute53.DeleteCNAMERecord(r); err != nil {
			d.logger.Errorf("failed to delete CNAME record for %s, err: %v", r, err)
			return err
		}
	}

	// FIXME Remove DNS records of host's containers, but don't fail if any record does not exist.

	return nil
}

// OnContainerCreate creates DNS records needed when container is created:
// 1. CNAME <container-slug>.PREVIEW_HOSTNAME to Tender server for the host's region.
//    e.g. CNAME rails-1.nitrousapp.com to amazon.nitrousproxy-sgp.com
func (d *RealDNS) OnContainerCreate(cn *container.Container, h *host.Host) error {
	sourceDomain := cn.PreviewHostname()
	targetDomain, err := h.ProxyTargetDomain()
	if err != nil {
		return err
	}

	if err := nroute53.SetCNAME(sourceDomain, targetDomain); err != nil {
		d.logger.Errorf("failed to set CNAME from %s to %s, err: %v", sourceDomain, targetDomain, err)
		return err
	}
	return nil
}

// onContainerDestroy deletes DNS records after container has been destroyed:
// 1. Remove CNAME to Tender server.
func (d *RealDNS) OnContainerDestroy(cn *container.Container) error {
	if err := nroute53.DeleteCNAMERecord(cn.PreviewHostname()); err != nil {
		d.logger.Errorf("failed to delete CNAME record for %s, err: %v", cn.PreviewHostname(), err)
		return err
	}
	return nil
}

func SetActiveDNS(active DNS) {
	activeDNS = active
}

// setHostProxyCNAME adds or updates CNAME record for the host's proxy.
// Source domain is <host-slug>.PROXY_HOSTNAME, target domain is the Tender
// server for the host's region.
func (d *RealDNS) setHostProxyCNAME(h *host.Host) error {
	sourceDomain := h.ProxyHostname()
	targetDomain, err := h.ProxyTargetDomain()
	if err != nil {
		return err
	}

	if err := nroute53.SetCNAME(sourceDomain, targetDomain); err != nil {
		d.logger.Errorf("failed to set CNAME from %s to %s, err: %v", sourceDomain, targetDomain, err)
		return err
	}
	return nil
}

// setHostCNAME adds or updates CNAME record for the host.
// Source domain is <host-slug>.HOST_HOSTNAME, target domain is the host's
// public domain.
func (d *RealDNS) setHostCNAME(h *host.Host) error {
	sourceDomain := h.Hostname()
	targetDomain, err := h.HostTargetDomain()
	if err != nil {
		return err
	}

	if err := nroute53.SetCNAME(sourceDomain, targetDomain); err != nil {
		d.logger.Errorf("failed to set CNAME from %s to %s, err: %v", sourceDomain, targetDomain, err)
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
	return activeDNS.OnContainerCreate(cn, h)
}

func OnContainerDestroy(cn *container.Container) error {
	return activeDNS.OnContainerDestroy(cn)
}
