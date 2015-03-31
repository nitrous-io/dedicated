package appconfig

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

const (
	defaultConfigPath = "/etc/default/nitrousd"

	DBEnvVar       = "NITROUS_PG_URL"
	MixpanelEnvVar = "MIXPANEL_TOKEN"
)

// config is used to decode JSON config files.
type config struct {
	// Set to true to enable debug functionality. DO NOT enable in production!
	Debug bool `json:"debug"`

	AESKey              string `json:"aes_key"`
	DatabaseURL         string `json:"database_url"`
	HostbuilderURL      string `json:"hostbuilder_url"`
	DefaultProvider     string `json:"default_provider"`
	DefaultAptLocation  string `json:"default_apt_location"`
	DefaultAptDist      string `json:"default_apt_dist"`
	CACertPath          string `json:"ca_cert_path"`
	CAKeyPath           string `json:"ca_key_path"`
	HostHostname        string `json:"host_hostname"`
	ProxyHostname       string `json:"proxy_hostname"`
	PreviewHostname     string `json:"preview_hostname"`
	WebHostname         string `json:"web_hostname"`
	FreighterdVersion   string `json:"freighterd_version"`
	DockerVersion       string `json:"docker_version"`
	ProvisionerUser     string `json:"provisioner_user"`
	ProvisionerInterval string `json:"provisioner_interval"`

	// Controls whether 'update now' option is available to users.
	EnableSelfUpgrade bool `json:"enable_self_upgrade"`

	Librato struct {
		User     string `json:"user"`
		Token    string `json:"token"`
		Interval string `json:"interval"`
	} `json:"librato"`

	Mixpanel struct {
		Token string `json:"token"`
	} `json:"mixpanel"`

	Sendgrid struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"sendgrid"`

	Stripe struct {
		SecretKey string `json:"secret_key"`
		PublicKey string `json:"public_key"`
	} `json:"stripe"`

	Github struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"github"`
}

var (
	AESKey             string
	Debug              bool
	DefaultAptDist     string
	DefaultAptLocation string
	DefaultProvider    string
	DockerVersion      string
	FreighterdVersion  string
	HostBuilderURL     string
	HostHostname       string
	MixpanelToken      string
	NitrousPgUrl       string
	ProxyHostname      string
	PreviewHostname    string
	WebHostname        string

	EnableSelfUpgrade bool

	CAKeyPath  string
	CACertPath string

	LibratoUser     string
	LibratoToken    string
	LibratoInterval string

	SendgridUsername string
	SendgridPassword string

	StripePublicKey string
	StripeSecretKey string

	ProvisionerUser     string
	ProvisionerInterval string

	ActivationEmailInterval string

	GetStartedEmailIntervalFirst  string
	GetStartedEmailIntervalSecond string
	GetStartedEmailIntervalThird  string

	GitHubOAuthClientID     string
	GitHubOAuthClientSecret string
)

func init() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		log.Printf("appconfig: CONFIG_PATH not defined, using default at %v", defaultConfigPath)
		configPath = defaultConfigPath
	}
	b, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalf("appconfig: failed to read config file, error: %v", err)
	}

	var cfg config
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&cfg); err != nil {
		log.Fatalf("appconfig: failed to parse config file, error: %v", err)
	}

	Debug = cfg.Debug
	AESKey = cfg.AESKey
	NitrousPgUrl = cfg.DatabaseURL
	DefaultAptLocation = cfg.DefaultAptLocation
	DefaultAptDist = cfg.DefaultAptDist
	DefaultProvider = cfg.DefaultProvider
	CAKeyPath = cfg.CAKeyPath
	CACertPath = cfg.CACertPath
	FreighterdVersion = cfg.FreighterdVersion
	DockerVersion = cfg.DockerVersion
	HostBuilderURL = cfg.HostbuilderURL
	HostHostname = cfg.HostHostname
	ProxyHostname = cfg.ProxyHostname
	PreviewHostname = cfg.PreviewHostname
	WebHostname = cfg.WebHostname
	ProvisionerUser = cfg.ProvisionerUser
	ProvisionerInterval = cfg.ProvisionerInterval
	EnableSelfUpgrade = cfg.EnableSelfUpgrade
	MixpanelToken = cfg.Mixpanel.Token
	LibratoUser = cfg.Librato.User
	LibratoToken = cfg.Librato.Token
	LibratoInterval = cfg.Librato.Interval
	SendgridUsername = cfg.Sendgrid.Username
	SendgridPassword = cfg.Sendgrid.Password
	StripePublicKey = cfg.Stripe.PublicKey
	StripeSecretKey = cfg.Stripe.SecretKey
	GitHubOAuthClientID = cfg.Github.ClientID
	GitHubOAuthClientSecret = cfg.Github.ClientSecret
}

func CheckCAVariables() {
	if CAKeyPath == "" {
		log.Fatalf("CA_KEY_PATH is not set")
	}
	if CACertPath == "" {
		log.Fatalf("CA_CERT_PATH is not set")
	}
}

func CheckSendgridVariables() {
	if SendgridUsername == "" {
		log.Fatalf("SENDGRID_USERNAME not set")
	} else if strings.HasPrefix(SendgridUsername, "disable") {
		log.Println("Warning: Sendgrid mailer disabled.")
		return
	}
	if SendgridPassword == "" {
		log.Fatalf("SENDGRID_PASSWORD not set")
	}
}

func CheckHostnames() {
	if HostHostname == "" {
		log.Fatal("HOST_HOSTNAME not set")
	}
	if ProxyHostname == "" {
		log.Fatal("PROXY_HOSTNAME not set")
	}
	if PreviewHostname == "" {
		log.Fatal("PREVIEW_HOSTNAME not set")
	}
}

func CheckGitHubOAuth() {
	if GitHubOAuthClientID == "" {
		log.Println("Warning: GITHUB_CLIENT_ID not set")
	}
	if GitHubOAuthClientSecret == "" {
		log.Println("Warning: GITHUB_CLIENT_SECRET not set")
	}
}
