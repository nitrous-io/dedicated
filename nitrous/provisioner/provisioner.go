package provisioner

import (
	"crypto/rsa"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"sync/atomic"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/nitrous-common/logger"
)

var HostbuilderClient *hostbuilderclient.Hostbuilder
var aesKey string
var defaultAPTLocation string
var defaultAPTDist string

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
// It is used for unmarshalling JSON of the form:
// {
//   "username": "host-provisioner",
//   "num_workers": "2",
//   "preprovision": [
//     {
//       "driver": "digitalocean",
//       "region": "sgp",
//       "plan": "1gb",
//       "count": 2
//     },
//     {
//       "driver": "docker",
//       "region": "sgp",
//       "plan": "512mb",
//       "count": 1
//     },
//   ]
// }
type Config struct {
	Username          string             `json:"username"`
	NumWorkers        int                `json:"num_workers"`
	PreprovisionSpecs []PreprovisionSpec `json:"preprovision,omitempty"`
}

type PreprovisionSpec struct {
	Driver string `json:"driver"`
	Region string `json:"region"`
	Plan   string `json:"plan"`
	Count  int    `json:"count"`
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

	AptLocation string
	AptDist     string

	ProvisionSucceededFn func(*host.Host) // FIXME Better way to do this? Better names?
	ProvisionFailedFn    func(*host.Host, error)
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

func init() {
	hostbuilderURL := os.Getenv("HOSTBUILDER_URL")
	if hostbuilderURL == "" {
		panic("provisioner: HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		panic("provisioner: AES_KEY is not set")
	}

	defaultAPTLocation = os.Getenv("DEFAULT_APT_LOCATION")
	if defaultAPTLocation == "" {
		panic("DEFAULT_APT_LOCATION is not set")
	}

	defaultAPTDist = os.Getenv("DEFAULT_APT_DIST")
	if defaultAPTDist == "" {
		panic("DEFAULT_APT_DIST is not set")
	}
}

func NewProvisioner(d *db.Database, l *logger.Logger, e *pubsub.Pubsub) *Provisioner {
	cfg, err := loadConfig()
	if err != nil {
		// FIXME Actually disable pre-provisioner using a flag or alternatively
		// return an error.
		l.Warnf("provisioner: failed to load config, disabling pre-provisioner: %s", err)
	}

	u, err := user.FindByUsername(d, cfg.Username)
	if err != nil {
		l.Warnf("provisioner: failed to load pre-provision user %q: %s", cfg.Username, err)
	}
	t := &token.Token{UserID: u.ID}
	if err := t.Insert(d); err != nil {
		l.Warnf("provisioner: failed to create pre-provision user's token: %s", err)
	}

	return &Provisioner{
		config:                cfg,
		db:                    d,
		logger:                l,
		events:                e,
		stop:                  make(chan struct{}),
		stopped:               make(chan struct{}),
		preprovisionUser:      u,
		preprovisionUserToken: t,
	}
}

// Start kicks off a goroutine that compares the required number of
// pre-provisioned hosts (specified by Provisioner.config) against reality.
// More hosts are provisioned as necessary.
func (p *Provisioner) Start() error {
	p.logger.Debug("provisioner: started")
	p.setRunning(true)

	ticker := time.NewTicker(p.pollInterval())
	go func() {
		defer p.setRunning(false)
		for {
			select {
			case <-ticker.C:
				p.logger.Debugf("provisioner: checking pre-provisioning req against reality...")

				for _, spec := range p.config.PreprovisionSpecs {
					if err := p.comparePreprovisioned(spec); err != nil {
						p.logger.Errorf("provisioner: pre-provisioning failed %s", err)
					}
				}
			case <-p.stop:
				ticker.Stop()
				p.logger.Info("provisioner: stopped")
				p.stopped <- struct{}{}
				return
			}
		}
	}()

	return nil
}

func (p *Provisioner) Stop() {
	p.stop <- struct{}{} // Don't close() channel so that we can start again.
	<-p.stopped
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
	// FIXME Check if pre-provisioned hosts are available, and if so, assign and
	// return one of them.

	if opts.AptLocation == "" {
		opts.AptLocation = defaultAPTLocation
	}
	if opts.AptDist == "" {
		opts.AptDist = defaultAPTDist
	}

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
	}
	if err := h.Validate(); err != nil {
		return nil, &InvalidHostError{Err: err}
	}

	// Generate Certificate Authority info.
	caInfo, err := certauth.GetCAInfo(true)
	if err != nil {
		return nil, err
	}

	// Save record to db first to reserve slug (since it's globally unique).
	tx, err := p.db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("provisioner: failed to begin db transaction for host %s, err: %s", h.Slug, err)
	}

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
		return nil, fmt.Errorf("provisioner: failed to save new host record %s, err: %s", h.Slug, err)
	}

	// Create host certificate and key. The cert will be used for TLS encryption
	// when communicating with the host.
	cert, err := cert.Generate(h.Hostname(), caInfo.X509Cert, caInfo.Key)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("provisioner: failed to generate certificates for '%s', err %s", h.Slug, err)
	}
	if err := cert.Insert(tx, h.ID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("provisioner: failed to save certificates for '%s', err %s", h.Slug, err)
	}

	// Create a keypair so nitrousd can communicate with the host.
	privKey, pubKey, err := generateKey()
	if err != nil {
		return nil, fmt.Errorf("provisioner: failed to generate keypair for '%s', err: %s", h.Slug, err)
	}
	if _, err := keypairs.Create(tx, privKey, aesKey, h.ID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("provisioner: failed to save keypair for '%s', err %s", h.Slug, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("provisioner: failed to complete save transaction for '%s', err %s", h.Slug, err)
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
			return
		}
		h.State = host.StateOrphan
		_ = h.Update(p.db) // Ignore error here.

		if opts.ProvisionFailedFn != nil {
			opts.ProvisionFailedFn(h, err)
		}
	}()

	config := p.mkHostbuilderConfig(phost, opts)
	hostinfo, err := HostbuilderClient.Create(opts.Template.Provider, &config, opts.OutputStream)
	if err != nil {
		return nil, err
	}

	h.IPAddress = sql.NullString{hostinfo.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(hostinfo.FreighterdPort), true}
	h.ProviderIdentifier = sql.NullString{hostinfo.ID, true}
	h.State = host.StateRunning

	if err = h.Update(p.db); err != nil {
		return nil, err
	}

	if opts.ProvisionSucceededFn != nil {
		opts.ProvisionSucceededFn(h)
	}

	return h, nil
}

func (p *Provisioner) preprovision() {
	// See https://github.com/docker/docker/pull/8570 for renaming of
	// Docker container name.
}

// comparePreprovisioned compares a PreprovisionSpec against actually
// pre-provisioned hosts. If any hosts are needed, they are pre-provisioned.
func (p *Provisioner) comparePreprovisioned(spec PreprovisionSpec) error {
	db := p.db
	// FIXME We should have a lock around the comparison so that we don't
	// pre-provision more hosts due to a race.
	p.logger.Debugf("provisioner: checking preprovisioning spec: %+v", spec)

	r, err := region.FindBySlug(db, spec.Region)
	if err != nil {
		return err
	}
	pl, err := plan.FindBySlug(db, spec.Plan)
	if err != nil {
		return err
	}
	template, err := providertemplate.FindByRegionAndPlanAndProvider(db, r.ID, pl.ID, spec.Driver)
	if err != nil {
		return err
	}
	hosts, err := host.FindAllPreprovisioned(db, r, pl)
	if err != nil {
		return err
	}

	needed := spec.Count - len(hosts)
	i := 1
	for i <= needed {
		p.logger.Debugf("provisioner: pre-provisioning %d of %d hosts", i, needed)

		// FIXME This should *NOT* call ProvisionHost because we want to save the
		// host to the preprovisioned_hosts table.

		// FIXME Do the pre-provisioning in a goroutine.
		// FIXME Record (with a mutex) that pre-provisioning is in progress.
		opts := ProvisionOptions{
			Async:        false,
			OutputStream: ioutil.Discard,
			Slug:         fmt.Sprintf("pre-provisioned-%d", rand.Int31n(2147483647)), // FIXME Generate a UUID.
			Name:         "Pre-provisioned host",
			User:         p.preprovisionUser,
			Token:        p.preprovisionUserToken,
			Region:       r,
			Plan:         pl,
			Template:     template,
			AptLocation:  defaultAPTLocation,
			AptDist:      defaultAPTDist,
		}
		h, err := p.ProvisionHost(opts)
		if err != nil {
			p.logger.Errorf("provisioner: failed to pre-provision host with opts %+v, err: %s", opts, err)
			return err
		}
		fmt.Printf("! new host pre-provisioned: %+v", h)
		i = i + 1
	}
	return nil
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
