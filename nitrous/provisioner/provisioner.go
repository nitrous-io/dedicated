package provisioner

import (
	"crypto/rsa"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/nitrous-common/logger"
)

var HostbuilderClient *hostbuilderclient.Hostbuilder
var aesKey string
var defaultAPTLocation string
var defaultAPTDist string
var fv, dv string

var (
	ErrPreprovisionerNotConfigured    = errors.New("pre-provisioner is not configured")
	ErrMultipleHostCreationNotAllowed = errors.New("only one host can be created at any time")
	ErrHostCreationNotAllowed         = errors.New("not allowed to create more hosts")
	ErrSlugIsTaken                    = errors.New("slug is taken")

	validations = map[string]error{
		"index_hosts_on_slug": ErrSlugIsTaken,
	}
)

type Provisioner struct {
	db        *db.Database
	logger    *logger.Logger
	events    *pubsub.Pubsub
	stop      chan struct{}
	stopped   chan struct{}
	isRunning uint32 // atomic bool, non zero if true

	config                *Config
	preprovisionUser      *user.User
	preprovisionUserToken *token.Token
}

// Config is the configuration of a host provisioner.
type Config struct {
	Username string
	Interval time.Duration
}

type ProvisionOptions struct {
	Async        bool
	Preprovision bool
	OutputStream io.Writer

	Slug     string
	Name     string
	User     *user.User
	Token    *token.Token
	Region   *region.Region
	Plan     *plan.Plan
	Template *providertemplate.ProviderTemplate

	UUID string

	AptLocation string
	AptDist     string

	ProvisionSucceededFn func(*host.Host)
	ProvisionFailedFn    func(*host.Host, error)
}

// job is a request to pre-provision a single host.
type job struct {
	template *providertemplate.ProviderTemplate
}

type preparedHost struct {
	host   *host.Host
	cert   *cert.Cert
	caInfo *certauth.CAInfo
	pubkey []byte
}

type InvalidHostError struct {
	Err error
}

func (e *InvalidHostError) Error() string {
	return fmt.Sprintf("provisioner: host is invalid: %s", e.Err.Error())
}

type HostbuilderError struct {
	Err error
}

func (e *HostbuilderError) Error() string {
	return fmt.Sprintf("provisioner: hostbuilder returned error: %s", e.Err.Error())
}

func init() {
	hostbuilderURL := envvars.HostBuilderURL
	if hostbuilderURL == "" {
		panic("provisioner: HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	aesKey = envvars.AESKey
	if aesKey == "" {
		panic("provisioner: AES_KEY is not set")
	}

	defaultAPTLocation = envvars.DefaultAptLocation
	if defaultAPTLocation == "" {
		panic("DEFAULT_APT_LOCATION is not set")
	}

	defaultAPTDist = envvars.DefaultAptDist
	if defaultAPTDist == "" {
		panic("DEFAULT_APT_DIST is not set")
	}

	fv = envvars.FreighterdVersion
	if fv == "" {
		panic("provisioner: FREIGHTERD_VERSION is not set")
	}

	dv = envvars.DockerVersion
	if dv == "" {
		panic("provisioner: DOCKER_VERSION is not set")
	}
}

func NewProvisioner(d *db.Database, l *logger.Logger, e *pubsub.Pubsub, conf *Config) *Provisioner {
	return &Provisioner{
		db:      d,
		logger:  l,
		events:  e,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		config:  conf,
	}
}

// Start kicks off a goroutine that pre-provisions hosts.
func (p *Provisioner) Start() error {
	if p.IsRunning() {
		p.logger.Info("provisioner: Start() called but already started")
		return nil
	}

	if p.config == nil {
		return ErrPreprovisionerNotConfigured
	}

	if err := p.configureUser(); err != nil {
		return err
	}

	p.stop = make(chan struct{})
	p.setRunning(true)

	p.logger.Debugf("provisioner: starting with config %+v", p.config)

	// Periodically determine if we need to pre-provision any hosts.
	errCh := make(chan error)
	go func() {
		db := p.db
		ticker := time.NewTicker(p.config.Interval)
		for {
			select {
			case <-ticker.C:
				ticker.Stop()
				pts, err := providertemplate.Preprovisioned(db)
				if err != nil {
					errCh <- err
					return
				}

				for _, pt := range pts {
					r := &region.Region{ID: pt.RegionID}
					// Skip if there are pre-provisioned orphan hosts in this region.
					numOrphans, err := host.CountPreprovisionedOrphans(db, r)
					if err != nil {
						errCh <- err
						return
					}
					if numOrphans > 0 {
						p.logger.Warnf("provisioner: %d orphan(s) in region %d, skipping job",
							numOrphans, pt.RegionID)
						continue
					}

					numHosts, err := host.CountPreprovisioned(db, r, &plan.Plan{ID: pt.PlanID})
					if err != nil {
						errCh <- err
						return
					}
					logPrefix := fmt.Sprintf("provisioner: host count for (template,region,plan) (%d,%d,%d) is %d, max is %d:",
						pt.ID, pt.RegionID, pt.PlanID, numHosts, pt.PreprovisionMax)
					needed := pt.PreprovisionMax - numHosts
					if needed <= 0 {
						p.logger.Debugf("%s skipping", logPrefix)
						continue
					}
					p.logger.Debugf("%s creating job", logPrefix)

					j := job{template: &pt}
					h, err := p.preprovision(j)
					if err != nil {
						p.logger.Errorf("provisioner: job %s/%d/%d failed, err: %v",
							j.template.Provider, j.template.RegionID, j.template.PlanID, err)
						continue
					}

					p.logger.Infof("provisioner: job %s/%d/%d successfully pre-provisioned host ID: %d",
						j.template.Provider, j.template.RegionID, j.template.PlanID, h.ID)
				}

				// Reset ticker.
				ticker = time.NewTicker(p.config.Interval)
			case <-p.stop:
				p.logger.Debug("provisioner: job creator quitting")
				ticker.Stop()
				return
			}
		}
	}()

	// Stop pre-provisioner goroutine on any errors.
	go func() {
		err := <-errCh
		p.logger.Errorf("provisioner: job creator encountered error: %s", err)
		p.Stop()
	}()

	// Process stop signal.
	go func() {
		<-p.stop
		p.setRunning(false)
		p.logger.Info("provisioner: stopped")
		p.stopped <- struct{}{}
	}()

	p.logger.Debug("provisioner: started")

	return nil
}

func (p *Provisioner) Stop() {
	if p.IsRunning() {
		close(p.stop)
		<-p.stopped
	}

	// Allow pre-provisioner to be started again with SIGUSR2.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGUSR2)
		<-ch
		p.logger.Infof("provisioner: SIGUSR2 received, re-starting")
		p.Start()
	}()
}

func (p *Provisioner) IsRunning() bool {
	return atomic.LoadUint32(&p.isRunning) > 0
}

func (p *Provisioner) setRunning(r bool) bool {
	if r {
		return atomic.CompareAndSwapUint32(&p.isRunning, 0, 1)
	} else {
		return atomic.CompareAndSwapUint32(&p.isRunning, 1, 0)
	}
}

// ProvisionHost creates a new Host record (and its associated records),
// provisions a host, and finally returns the Host record with updated data
// from the provisioning step.
// If the Async option is true, the current Host record is returned immediately
// while provisioning happens asynchonously.
func (p *Provisioner) ProvisionHost(opts ProvisionOptions) (*host.Host, error) {
	if opts.AptLocation == "" {
		opts.AptLocation = defaultAPTLocation
	}
	if opts.AptDist == "" {
		opts.AptDist = defaultAPTDist
	}

	// Make sure user is allowed to create more hosts.
	canCreateHosts, err := opts.User.CanCreateHosts(p.db)
	if !canCreateHosts {
		return nil, ErrHostCreationNotAllowed
	}

	// Assign a pre-provisioned host if one is available.
	preprovisioned, err := host.FindPreprovisioned(p.db, opts.Region, opts.Plan)
	if err == nil {
		return p.usePreprovisionedHost(preprovisioned, opts)
	}

	return p.provisionNewHost(opts)
}

var (
	assignments = make(map[int64]bool)
	mutex       = &sync.Mutex{}
)

// markAssignment records that a pre-provisioned host is in the process of being
// assigned to a real user and returns true if successful. A mutex is used so
// that we can safely assign hosts across multiple goroutines.
func markAssignment(h *host.Host) bool {
	mutex.Lock()
	defer mutex.Unlock()
	if assignments[h.ID] {
		return false
	}
	assignments[h.ID] = true
	return true
}

func unmarkAssignment(h *host.Host) {
	mutex.Lock()
	assignments[h.ID] = false
	mutex.Unlock()
}

func (p *Provisioner) usePreprovisionedHost(h *host.Host, opts ProvisionOptions) (*host.Host, error) {
	assignedHost, err := p.assignHost(h, opts)
	if err != nil {
		p.logger.Warnf("provisioner: could not assign pre-provisioned host %d, err: %v", h.ID, err)
		return p.provisionNewHost(opts)
	}
	return assignedHost, nil
}

func (p *Provisioner) provisionNewHost(opts ProvisionOptions) (*host.Host, error) {
	phost, err := p.prepare(opts)
	if err != nil {
		return nil, err
	}

	if opts.Async {
		go p.provision(phost, opts)
		return phost.host, nil
	}

	return p.provision(phost, opts)
}

func (p *Provisioner) assignHost(preprovisionedHost *host.Host, opts ProvisionOptions) (h *host.Host, err error) {
	if !markAssignment(preprovisionedHost) {
		return nil, fmt.Errorf("provisioner: host %d became assigned while we weren't looking", preprovisionedHost.ID)
	}
	defer unmarkAssignment(preprovisionedHost)

	cp := *preprovisionedHost // Copy of function arg.
	h = &cp

	defer func() {
		if err == nil {
			if opts.ProvisionSucceededFn != nil {
				opts.ProvisionSucceededFn(h)
			}
		} else {
			if opts.ProvisionFailedFn != nil {
				opts.ProvisionFailedFn(h, err)
			}
		}
	}()

	// Update host in hostbuilder (e.g. rename host).
	config := &hostbuilderclient.Config{
		"id":           preprovisionedHost.ProviderIdentifier.String,
		"name":         opts.Slug,
		"apt_username": opts.User.Username,
		"apt_password": opts.Token.Token,
		"apt_location": opts.AptLocation,
		"apt_dist":     opts.AptDist,
	}
	hostinfo, err := HostbuilderClient.Update(opts.Template.Provider, config, opts.OutputStream, p.hostbuilderHeaders(opts))
	if err != nil {
		err = fmt.Errorf("provisioner: failed to update host in Hostbuilder, err: %v", err)
		return
	}

	// Update host record with new info.
	h.Slug = opts.Slug
	h.Name = sql.NullString{opts.Name, true}
	h.IPAddress = sql.NullString{hostinfo.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(hostinfo.FreighterdPort), true}
	h.ProviderIdentifier = sql.NullString{hostinfo.ID, true}
	h.HourlyPrice = opts.Plan.HourlyPrice()
	h.Preprovisioned = false
	if err = h.Update(p.db); err != nil {
		err = fmt.Errorf("provisioner: failed to update pre-provisioned host record, err: %v", err)
		return
	}

	if err = h.Reassign(p.db, opts.User); err != nil {
		err = fmt.Errorf("provisioner: failed to assign pre-provisioned host %d to real user, err: %v", h.ID, err)
		return
	}

	return
}

// prepare creates a new Host and its associated certificates and keys in
// preparation for actual provisioning.
func (p *Provisioner) prepare(opts ProvisionOptions) (*preparedHost, error) {
	h := &host.Host{
		UserID:             opts.User.ID,
		RegionID:           opts.Region.ID,
		PlanID:             opts.Plan.ID,
		ProviderTemplateID: opts.Template.ID,
		Slug:               opts.Slug,
		Name:               sql.NullString{opts.Name, true},
		MaxContainers:      sql.NullInt64{opts.Plan.MaxContainers, true},
		Size:               sql.NullString{opts.Plan.Slug, true},
		State:              host.StateBorn,
		Preprovisioned:     opts.Preprovision,
		FreighterdVersion:  fv,
		DockerVersion:      dv,
	}
	if err := h.Validate(); err != nil {
		return nil, &InvalidHostError{Err: err}
	}

	// Generate Certificate Authority info.
	caInfo, err := certauth.NewCAInfo(envvars.CACertPath, envvars.CAKeyPath)
	if err != nil {
		return nil, err
	}

	// Save record to db first to reserve slug (since it's globally unique).
	tx, err := p.db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("provisioner: failed to begin db transaction for host %s, err: %v", h.Slug, err)
	}

	if !opts.Preprovision {
		// Don't allow hosts to be created concurrently.
		locked, err := db.TransactionLock(tx, db.HOST_CREATE_LOCK, int(opts.User.ID))
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if !locked {
			tx.Rollback()
			return nil, ErrMultipleHostCreationNotAllowed
		}
	}

	// Make sure user is allowed to create more hosts.
	canCreateHosts, err := opts.User.CanCreateHosts(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if !canCreateHosts {
		tx.Rollback()
		return nil, ErrHostCreationNotAllowed
	}

	if err := h.Insert(tx); err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if ne, ok := validations[e.Constraint]; ok {
				return nil, ne
			}
		}
		return nil, fmt.Errorf("provisioner: failed to save new host record %s, err: %v", h.Slug, err)
	}

	// Create host certificate and key. The cert will be used for TLS encryption
	// when communicating with the host.
	cert, err := cert.Generate(h.CertHostname(), caInfo.X509Cert, caInfo.Key)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("provisioner: failed to generate certificates for '%s', err: %v", h.Slug, err)
	}
	if err := cert.Insert(tx, h.ID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("provisioner: failed to save certificates for '%s', err: %v", h.Slug, err)
	}

	// Create a keypair so nitrousd can communicate with the host.
	privKey, pubKey, err := generateKey()
	if err != nil {
		return nil, fmt.Errorf("provisioner: failed to generate keypair for '%s', err: %v", h.Slug, err)
	}
	if _, err := keypairs.Create(tx, privKey, aesKey, h.ID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("provisioner: failed to save keypair for '%s', err: %v", h.Slug, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("provisioner: failed to complete save transaction for '%s', err: %v", h.Slug, err)
	}

	return &preparedHost{
		host:   h,
		cert:   cert,
		caInfo: caInfo,
		pubkey: pubKey,
	}, nil
}

// provision requests hostbuilder to create a host with the given data and
// options.
func (p *Provisioner) provision(phost *preparedHost, opts ProvisionOptions) (*host.Host, error) {
	h := phost.host

	var err error
	// If an error occurs at any point, mark host as orphan.
	defer func() {
		if err == nil {
			if opts.ProvisionSucceededFn != nil {
				opts.ProvisionSucceededFn(h)
			}
		} else {
			h.State = host.StateOrphan
			_ = h.Update(p.db) // Ignore error here.
			if opts.ProvisionFailedFn != nil {
				opts.ProvisionFailedFn(h, err)
			}
		}
	}()

	config := p.mkHostbuilderConfig(phost, opts)
	hostinfo, err := HostbuilderClient.Create(opts.Template.Provider, &config, opts.OutputStream, p.hostbuilderHeaders(opts))
	if err != nil {
		err = &HostbuilderError{Err: err}
		return nil, err
	}

	h.IPAddress = sql.NullString{hostinfo.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(hostinfo.FreighterdPort), true}
	h.ProviderIdentifier = sql.NullString{hostinfo.ID, true}
	h.State = host.StateRunning
	h.HourlyPrice = opts.Plan.HourlyPrice()

	if err = h.Update(p.db); err != nil {
		return nil, err
	}
	return h, nil
}

func (p *Provisioner) configureUser() error {
	u, err := user.FindByUsername(p.db, p.config.Username)
	if err != nil {
		return err
	}
	if u == nil {
		return fmt.Errorf("User '%s' could not be found", p.config.Username)
	}
	t := &token.Token{UserID: u.ID, Type: token.TypeProvisioner}
	if err := t.Insert(p.db); err != nil {
		return err
	}
	p.preprovisionUser = u
	p.preprovisionUserToken = t
	return nil
}

func (p *Provisioner) preprovision(j job) (*host.Host, error) {
	r, err := region.FindByID(p.db, j.template.RegionID)
	if err != nil {
		return nil, err
	}
	pl, err := plan.FindByID(p.db, j.template.PlanID)
	if err != nil {
		return nil, err
	}

	slug := fmt.Sprintf("pre-provisioned-%d", time.Now().UnixNano())
	opts := ProvisionOptions{
		Async:        false,
		Preprovision: true,
		OutputStream: ioutil.Discard,
		Slug:         slug,
		Name:         "Pre-provisioned host",
		User:         p.preprovisionUser,
		Token:        p.preprovisionUserToken,
		Region:       r,
		Plan:         pl,
		Template:     j.template,
		AptLocation:  defaultAPTLocation,
		AptDist:      defaultAPTDist,
	}

	phost, err := p.prepare(opts)
	if err != nil {
		return nil, err
	}

	return p.provision(phost, opts)
}

func (p *Provisioner) mkHostbuilderConfig(phost *preparedHost, opts ProvisionOptions) hostbuilderclient.Config {
	h := phost.host
	config := hostbuilderclient.Config{
		"name":        h.Slug,
		"secret":      h.Secret,
		"public_keys": []string{string(phost.pubkey)},
		"tls": map[string]string{
			"ca":   string(phost.caInfo.CertPEM),
			"cert": phost.cert.ServerCert,
			"key":  phost.cert.ServerKey,
		},
		"apt_username": opts.User.Username,
		"apt_password": opts.Token.Token,
		"apt_location": opts.AptLocation,
		"apt_dist":     opts.AptDist,
	}
	for k, v := range opts.Template.Config.Map {
		if v.Valid {
			config[k] = v.String
		} else {
			config[k] = ""
		}
	}
	return config
}

func (p *Provisioner) hostbuilderHeaders(opts ProvisionOptions) map[string]string {
	return map[string]string{constants.NitrousUUIDHeader: opts.UUID}
}

// generateKey generates and returns a private key together with its public key.
func generateKey() (priv *rsa.PrivateKey, pub []byte, err error) {
	priv, err = keypairs.GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	pub, err = keypairs.PublicKey(&priv.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}
