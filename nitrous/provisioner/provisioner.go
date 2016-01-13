package provisioner

import (
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
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
	ErrOrgPermissionDenied            = errors.New("you do not have access to this organization")
	ErrOrgCreatePermissionDenied      = errors.New("limited user cannot create")
	ErrSlugIsTaken                    = errors.New("slug is taken")
	ErrUnknownProvider                = errors.New("unknown provider")
	ErrNoSubscriptionProvided         = errors.New("a subscription must be provided when MaxHosts is set on a Plan")
	ErrExceededPlanMaxHosts           = errors.New("you have exceeded the number of workstations allowed for this plan")

	validations = map[string]error{
		"index_hosts_on_slug": ErrSlugIsTaken,
	}
)

type Provisioner struct {
	db     *db.Database
	events *pubsub.Pubsub

	config *Config
	caInfo *certauth.CAInfo
}

// Config is the configuration of a host provisioner.
type Config struct {
	Username string
	Interval time.Duration
}

type ProvisionOptions struct {
	Async        bool
	OutputStream io.Writer

	Slug         string
	Name         string
	User         *user.User
	Region       *region.Region
	Plan         *plan.Plan
	Template     *providertemplate.ProviderTemplate
	Org          *org.Org
	Subscription *subscription.Subscription

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
		db:     d,
		events: e,
		config: conf,
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

	if opts.Org != nil {
		om, err := orgmembership.FindByIdentifierAndOrgSlug(p.db, opts.User.Username, opts.Org.Slug)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrOrgPermissionDenied
			}
			return nil, err
		}
		if om.Role == orgmembership.RoleLimited {
			return nil, ErrOrgCreatePermissionDenied
		}
	}

	// Make sure user is allowed to create more hosts.
	if err := p.canCreateHosts(opts); err != nil {
		return nil, err
	}

	return p.provisionNewHost(opts)
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
		Hotswap:            true, // TODO remove this field once hostbuilder no longer reads it
		Size:               sql.NullString{opts.Plan.Slug, true},
		State:              host.StateBorn,
		OrgID:              opts.OrgID(),
		// If the host's provider_template.config has disable_eip => true, static_ip
		// column should be false
		StaticIP: !getOptionDisableEIP(opts),
	}

	// only use an existing subscription if it supports MaxHosts
	if opts.Subscription != nil && opts.Subscription.MultiHost && opts.Plan.MaxHosts.Valid {
		h.SubscriptionID = sql.NullInt64{opts.Subscription.ID, true}
	}

	if err := h.Validate(); err != nil {
		return nil, &InvalidHostError{Err: err}
	}

	// Save record to db first to reserve slug (since it's globally unique).
	tx, err := p.db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("provisioner: failed to begin db transaction for host %s, err: %v", h.Slug, err)
	}

	// Don't allow hosts to be created concurrently.
	var bp *billingprofile.BillingProfile
	if opts.Org != nil {
		bp, err = billingprofile.FindForOrg(p.db, opts.Org.ID)
		if err != nil {
			return nil, err
		}
	} else {
		bp, err = opts.User.BillingProfile(p.db)
		if err != nil {
			return nil, err
		}
	}
	locked, err := db.TransactionLock(tx, db.HOST_CREATE_LOCK, int(bp.ID))
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if !locked {
		tx.Rollback()
		return nil, ErrMultipleHostCreationNotAllowed
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

func (p *Provisioner) mkHostbuilderConfig(phost *preparedHost, opts ProvisionOptions) hostbuilderclient.Config {
	h := phost.host
	config := hostbuilderclient.Config{
		"name":        h.Slug, // overridden by provider-specific names later
		"secret":      h.Secret,
		"public_keys": []string{string(phost.pubkey)},
		"tls": map[string]string{
			"ca":   string(phost.caInfo.CertPEM),
			"cert": phost.cert.ServerCert,
			"key":  phost.cert.ServerKey,
		},
		"apt_username": strconv.FormatInt(h.ID, 10),
		"apt_password": h.Secret,
		"apt_location": opts.AptLocation,
		"apt_dist":     opts.AptDist,
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
	if opts.Org != nil {
		err := opts.Org.CanCreateHosts(p.db)
		if err != nil {
			return err
		}
	} else if err := opts.User.CanCreateHosts(p.db); err != nil {
		return err
	}

	// check plan's MaxHosts has not been exceeded
	if opts.Plan.MaxHosts.Valid {
		if opts.Subscription == nil {
			return ErrNoSubscriptionProvided
		}
		var size int64
		if err := p.db.QueryRowx(`SELECT count(h.id)
			FROM hosts h
			WHERE subscription_id = $1 AND terminated_at IS NULL;`,
			opts.Subscription.ID).Scan(&size); err != nil {
			return err
		}
		if size >= opts.Plan.MaxHosts.Int64 {
			return ErrExceededPlanMaxHosts
		}
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
