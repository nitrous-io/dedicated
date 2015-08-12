package provisioner

import (
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/nitrous-common/cipher"
	"golang.org/x/crypto/ssh"
)

var (
	// shared client to interact with hostbuilder
	HostbuilderClient *hostbuilderclient.Hostbuilder
	// AES_KEY to encrypt generated private key for each host
	aesKey string
	// default debian apt repository that is sent to each host
	defaultAPTLocation string
	// default debian apt distribution that is sent to each host
	defaultAPTDist string
	// default freighter and docker version
	fv, dv string
	// shared CA information, generated from the provided cert/key path in ENV
	caInfo *certauth.CAInfo
)

var (
	ErrPreprovisionerNotConfigured    = errors.New("pre-provisioner is not configured")
	ErrMultipleHostCreationNotAllowed = errors.New("only one host can be created at any time")
	ErrHostCreationNotAllowed         = errors.New("not allowed to create more hosts")
	ErrOrgPermissionDenied            = errors.New("you do not have access to this organization")
	ErrSlugIsTaken                    = errors.New("slug is taken")
	ErrUnknownProvider                = errors.New("unknown provider")

	validations = map[string]error{
		"index_hosts_on_slug": ErrSlugIsTaken,
	}

	// default mapping for digitalocean regions
	doRegionsMap = map[string]string{
		"sgp": "sgp1",
		"nyc": "nyc3",
		"sfo": "sfo1",
		"ams": "ams3",
	}
)

type Provisioner struct {
	db      *db.Database
	events  *pubsub.Pubsub
	didStop chan struct{}

	muIsRunning sync.Mutex
	isRunning   bool

	config           *Config
	preprovisionUser *user.User
	caInfo           *certauth.CAInfo
}

// Config is the configuration of a host provisioner.
type Config struct {
	Username string
	Interval time.Duration
}

type ProvisionOptions struct {
	Async              bool
	Preprovision       bool
	BypassPreprovision bool
	OutputStream       io.Writer

	Slug     string
	Name     string
	User     *user.User
	Region   *region.Region
	Plan     *plan.Plan
	Template *providertemplate.ProviderTemplate
	Org      *org.Org

	UUID string

	AptLocation string
	AptDist     string

	ProvisionSucceededFn func(*host.Host)
	ProvisionFailedFn    func(*host.Host, error)
}

func (o *ProvisionOptions) OrgID() sql.NullInt64 {
	if o.Org == nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{o.Org.ID, true}
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
		logrus.Warn("FREIGHTERD_VERSION not set")
	}

	dv = envvars.DockerVersion
	if dv == "" {
		panic("provisioner: DOCKER_VERSION is not set")
	}

	// preloaded the caInfo at boot time, so we dont have to recalculate this
	// for every host since the result will be the same
	var err error
	caInfo, err = certauth.NewCAInfo(envvars.CACertPath, envvars.CAKeyPath)
	if err != nil {
		panic(fmt.Sprintf("provisioner: invalid CA cert %q and key %q, error: %v",
			envvars.CACertPath, envvars.CAKeyPath, err))
	}
}

func NewProvisioner(d *db.Database, e *pubsub.Pubsub, conf *Config) *Provisioner {
	return &Provisioner{
		db:      d,
		events:  e,
		didStop: make(chan struct{}),
		config:  conf,
	}
}

func (p *Provisioner) IsRunning() bool {
	p.muIsRunning.Lock()
	p.muIsRunning.Unlock()
	return p.isRunning
}

// Start kicks off a goroutine that pre-provisions hosts.
func (p *Provisioner) Start() error {

	if p.config == nil {
		return ErrPreprovisionerNotConfigured
	}

	p.muIsRunning.Lock()
	if p.isRunning {
		logrus.Info("provisioner: Start() called but already started")
		p.muIsRunning.Unlock()
		return nil
	}

	if err := p.configureUser(); err != nil {
		p.muIsRunning.Unlock()
		return err
	}

	p.didStop = make(chan struct{})
	p.isRunning = true
	p.muIsRunning.Unlock()

	// Allow provisioner to be restarted with SIGUSR2.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGUSR2)
		<-ch

		logrus.Infof("provisioner: SIGUSR2 received, restarting")
		p.Stop()
		// Replace hostbuilder client with a new one.
		hostbuilderURL := envvars.HostBuilderURL
		if hostbuilderURL == "" {
			panic("provisioner: HOSTBUILDER_URL is not set")
		}
		HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)
		p.Start()
	}()

	logrus.Debugf("provisioner: starting with config %+v", p.config)

	// Periodically determine if we need to pre-provision any hosts.
	go func() {
		db := p.db
		ticker := time.NewTicker(p.config.Interval)
		for {
			select {
			case <-ticker.C:
				ticker.Stop()
				pts, err := providertemplate.Preprovisioned(db)
				if err != nil {
					p.StopWithError(err)
					return
				}

				for _, pt := range pts {
					r := &region.Region{ID: pt.RegionID}
					// Skip if there are pre-provisioned orphan hosts in this region.
					numOrphans, err := host.CountPreprovisionedOrphans(db, r)
					if err != nil {
						p.StopWithError(err)
						return
					}
					if numOrphans > 0 {
						logrus.Debugf("provisioner: %d orphan(s) in region %d, skipping job",
							numOrphans, pt.RegionID)
						continue
					}

					numHosts, err := host.CountPreprovisioned(db, r, &plan.Plan{ID: pt.PlanID})
					if err != nil {
						p.StopWithError(err)
						return
					}
					logPrefix := fmt.Sprintf("provisioner: host count for (template,region,plan) (%d,%d,%d) is %d, max is %d:",
						pt.ID, pt.RegionID, pt.PlanID, numHosts, pt.PreprovisionMax)
					needed := pt.PreprovisionMax - numHosts
					if needed <= 0 {
						logrus.Debugf("%s skipping", logPrefix)
						continue
					}
					logrus.Debugf("%s creating job", logPrefix)

					j := job{template: &pt}

					h, err := p.preprovision(j)
					if err != nil {
						logrus.Errorf("provisioner: job %s/%d/%d failed, err: %v",
							j.template.Provider, j.template.RegionID, j.template.PlanID, err)
						continue
					}

					logrus.Infof("provisioner: job %s/%d/%d successfully pre-provisioned host ID: %d",
						j.template.Provider, j.template.RegionID, j.template.PlanID, h.ID)
				}

				ticker = time.NewTicker(p.config.Interval)
			case <-p.didStop:
				logrus.Debug("provisioner: job creator quitting")
				ticker.Stop()
				return
			}
		}
	}()

	logrus.Debug("provisioner: started")

	return nil
}

func (p *Provisioner) StopWithError(err error) {
	logrus.Errorf("provisioner: job creator encountered error: %s", err)
	p.Stop()
}

func (p *Provisioner) Stop() {
	p.muIsRunning.Lock()
	defer p.muIsRunning.Unlock()
	if !p.isRunning {
		return
	}
	p.isRunning = false
	logrus.Info("provisioner: stopped")
	close(p.didStop)
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

	if opts.Org != nil {
		_, err := orgmembership.FindByUsernameAndOrgSlug(p.db, opts.User.Username, opts.Org.Slug)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrOrgPermissionDenied
			}
			return nil, err
		}
	}

	// Make sure user is allowed to create more hosts.
	if err := p.canCreateHosts(opts); err != nil {
		return nil, err
	}

	if !opts.BypassPreprovision {
		// Assign a pre-provisioned host if one is available.
		preprovisioned, err := host.FindPreprovisioned(p.db, opts.Region, opts.Plan)
		if err == nil {
			return p.usePreprovisionedHost(preprovisioned, opts)
		} else if err != sql.ErrNoRows {
			logrus.Warnf("Error finding preprovisioned hosts: %s", err)
		}
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
		logrus.Warnf("provisioner: could not assign pre-provisioned host %d, err: %v", h.ID, err)
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
	config := preprovisionedHost.MachineConfig()
	config["host_id"] = h.ID
	config["name"] = opts.Slug
	config["apt_username"] = strconv.FormatInt(h.ID, 10)
	config["apt_password"] = h.Secret
	config["apt_location"] = opts.AptLocation
	config["apt_dist"] = opts.AptDist

	urlArgs := host.URLArgsFromHost(h)
	hostinfo, err := HostbuilderClient.Update(opts.Template.Provider, config, opts.OutputStream, p.hostbuilderHeaders(opts), urlArgs)
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			logrus.Errorf("hostbuilder timeout updating host: %v", e)
		}
		return nil, fmt.Errorf("provisioner: failed to update host in Hostbuilder, err: %v", err)
	}

	// Update host record with new info.
	h.Slug = opts.Slug
	h.Name = sql.NullString{opts.Name, true}
	h.IPAddress = sql.NullString{hostinfo.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(hostinfo.FreighterdPort), true}
	h.ProviderIdentifier = sql.NullString{hostinfo.ID, true}
	h.HourlyPrice = opts.Plan.HourlyPrice()
	h.Free = opts.Plan.IsFree()
	h.AutoShutdown = opts.Plan.AutoShutdown
	h.OrgID = opts.OrgID()
	h.Preprovisioned = false
	if err = h.UpdateWithPrice(p.db); err != nil {
		err = fmt.Errorf("provisioner: failed to update pre-provisioned host record, err: %v", err)
		return
	}

	if err = h.Reassign(p.db, opts.User); err != nil {
		err = fmt.Errorf("provisioner: failed to assign pre-provisioned host %d to real user, err: %v", h.ID, err)
		return
	}

	return
}

func getOptionDisableEIP(opts ProvisionOptions) bool {
	for k, v := range opts.Template.Config.Map {
		if k == "disable_eip" {
			if !v.Valid {
				return false
			}
			disableEIP, err := strconv.ParseBool(v.String)
			if err != nil {
				return false
			}
			return disableEIP
		}
	}
	return false
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
		AutoShutdown:       opts.Plan.AutoShutdown,
		Size:               sql.NullString{opts.Plan.Slug, true},
		State:              host.StateBorn,
		OrgID:              opts.OrgID(),
		Preprovisioned:     opts.Preprovision,
		FreighterdVersion:  fv,
		DockerVersion:      dv,
		// If the host's provider_template.config has disable_eip => true, static_ip
		// column should be false
		StaticIP: !getOptionDisableEIP(opts),
	}

	if err := h.Validate(); err != nil {
		return nil, &InvalidHostError{Err: err}
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

	if err := p.canCreateHosts(opts); err != nil {
		return nil, err
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
			if h == nil {
				fmt.Printf("Err: %+v\n", err)
			}
			h.State = host.StateOrphan
			// ignore, but log
			if err := p.updateProviderDetails(h); err != nil {
				logrus.Debugf("provisionTracer: defer-updateProviderDetails err: %+v h: %+v ", err, h)
			}
			if opts.ProvisionFailedFn != nil {
				opts.ProvisionFailedFn(h, err)
			}
		}
	}()

	config := p.mkHostbuilderConfig(phost, opts)
	urlArgs := host.URLArgsFromHost(h)
	hostinfo, err := HostbuilderClient.Create(opts.Template.Provider, config, opts.OutputStream, p.hostbuilderHeaders(opts), urlArgs)
	logrus.Debugf("provisionTracer: hostinfo: %+v err: %+v", hostinfo, err)
	if hostinfo != nil {
		h.IPAddress = sql.NullString{hostinfo.IPAddress, true}
		h.ProviderIdentifier = sql.NullString{hostinfo.ID, true}
		h.RegionIdentifier = sql.NullString{hostinfo.Region, true}
	}
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			logrus.Debugf("provisionTracer: hostbuilder timeout creating host: %v", e)
		}
		err = &HostbuilderError{Err: err}
		return nil, err
	}

	// refresh the host for live DB data
	// (assign, not return so the value is passed into defer)
	nH, err := host.FindByIdentifier(p.db, h.Slug)
	if nH != nil {
		h = nH
	}
	return h, nil
}

func (p *Provisioner) configureUser() error {
	u, err := user.FindByUsername(p.db, p.config.Username)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("User '%s' could not be found", p.config.Username)
		}
		return err
	}
	p.preprovisionUser = u
	return nil
}

func (p *Provisioner) preprovision(j job) (*host.Host, error) {
	r, err := region.FindByID(p.db, j.template.RegionID)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("region %d not found", j.template.RegionID)
	}
	if r.Disabled() {
		return nil, fmt.Errorf("region %d is disabled", r.ID)
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
		"host_id":     h.ID,
		"name":        h.Slug, // overridden by provider-specific names later
		"secret":      h.Secret,
		"public_keys": []string{string(phost.pubkey)},
		"tls": map[string]string{
			"ca":   string(phost.caInfo.CertPEM),
			"cert": phost.cert.ServerCert,
			"key":  phost.cert.ServerKey,
		},
		"apt_username":       strconv.FormatInt(h.ID, 10),
		"apt_password":       h.Secret,
		"apt_location":       opts.AptLocation,
		"apt_dist":           opts.AptDist,
		"freighterd_version": phost.host.FreighterdVersion,
	}

	h.ProviderTemplate = opts.Template
	machineCfg := h.MachineConfig()
	for k, v := range machineCfg {
		config[k] = v
	}
	return config
}

func (p *Provisioner) hostbuilderHeaders(opts ProvisionOptions) http.Header {
	h := http.Header{}
	h.Set(constants.NitrousUUIDHeader, opts.UUID)
	return h
}

func (p *Provisioner) updateProviderDetails(h *host.Host) error {
	nstmt, err := p.db.PrepareNamed(`UPDATE hosts
		SET (ip_address, provider_identifier, region_identifier, freighterd_port, state, updated_at)
		= (:ip_address, :provider_identifier, :region_identifier, :freighterd_port, :state, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (p *Provisioner) canCreateHosts(opts ProvisionOptions) error {
	var (
		canCreateHosts = false
		err            error
	)
	if opts.Org != nil {
		canCreateHosts, err = opts.Org.CanCreateHosts(p.db)
		if err != nil {
			return err
		}
	} else {
		canCreateHosts, err = opts.User.CanCreateHosts(p.db)
		if err != nil {
			return err
		}
	}

	if !canCreateHosts {
		return ErrHostCreationNotAllowed
	}

	return nil
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

// pemPrivateKey returns the host's PEM-encoded RSA private key.
func pemPrivateKey(h *host.Host, d *db.Database) ([]byte, error) {
	kp, err := keypairs.Get(d, h.ID)
	if err != nil {
		return nil, err
	}

	priv, err := cipher.Decrypt(
		[]byte(aesKey),
		[]byte(kp.EncryptedPrivateKeyIv),
		[]byte(kp.EncryptedPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key for host %d, err: %v", h.ID, err)
	}

	pk, err := ssh.ParseRawPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key for host %d, err: %v", h.ID, err)
	}
	privKey, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid RSA key for host %d", h.ID)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	}), nil
}
