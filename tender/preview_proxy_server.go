package tender

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"

	_ "github.com/lib/pq"
	"github.com/nitrous-io/dedicated/freighter/config"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/pkg/proxy"
)

// PreviewProxyServer let user proxy preview requests to the host.
// The server is awared of the exposed ports on the host and proxy requests
// accordingly
type PreviewProxyServer struct {
	// direct access to database so we can fetch stuffs as fast as possible
	// FIXME:
	// - considering replace this with distributed datastore such as etcd
	db *sql.DB
	// expected DNS based domain for host's request. Request coming from this
	// domain will be proxied to freighterd server on that host.
	domain string
	// expected domain that is used when we create the TLS certificates in webapp
	// `nitrouspro.com`. This is used when communicating with the freighterd
	// server on the host.
	tlsDomain string
	// store the CA so we can build freighterclient
	ca []byte
}

// Returns a `http.Handler` which can be used to proxy preview request to
// a Nitrous.IO host
func NewPreviewProxyServer(baseDomain, tlsDomain string,
	db *sql.DB, caBytes []byte) (http.Handler, error) {
	s := &PreviewProxyServer{
		db:        db,
		domain:    baseDomain,
		tlsDomain: tlsDomain,
		ca:        caBytes,
	}
	return s, nil
}

func (s *PreviewProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := s.proxy(w, r); err != nil {
		handleError(w, err)
		return
	}
}

func (s *PreviewProxyServer) proxy(w http.ResponseWriter, r *http.Request) (err error) {
	name, err := getHost(r.Host, s.domain)
	if err != nil {
		return err
	}
	log.Printf("[INFO] request %v to %s", r, name)

	// TODO: since this will likely never change, cache this here for
	// a long time i.e. (server duration ?)
	container, err := findContainer(s.db, name)
	if err != nil {
		return err
	}

	// TODO: cache this data so we dont have to make so many roundtrips
	fhost := freighterclient.NewHost(
		container.HostIP.String(),
		container.HostPort,
		fmt.Sprintf("%s%s", container.HostSlug, s.tlsDomain),
		s.ca,
	)
	fclient, err := freighterclient.NewClient(fhost, container.HostSecret)
	if err != nil {
		return fmt.Errorf("failed to initialize freighter client for %s: %v", container.Slug, err)
	}
	info, err := fclient.Info(container.CID)
	if err != nil {
		return fmt.Errorf("failed to fetch container info %s: %v", container.Slug, err)
	}

	// set X-FORWARDED-PORT for the request
	fp := strconv.Itoa(parsePort(r))
	r.Header.Set(config.ForwardPortHeader, fp)
	publicPort, err := getPublicFreighterdPort(info.Ports, fp)
	if err != nil {
		return err
	}

	dest := setRequestHost(r, container.HostIP, publicPort)

	if proxy.IsWebsocket(r) {
		proxy.ProxyWebsocket(dest, nil)(w, r)
	} else {
		proxy.ProxyHTTP(dest, nil)(w, r)
	}
	return nil
}
