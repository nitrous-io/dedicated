package envvars

import (
	"log"
	"os"
	"strconv"
	"strings"
)

var (
	AESKey             string
	Debug              string
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

	// Controls whether 'update now' option is available to users.  Defaults to
	// true if ENABLE_SELF_UPGRADE is absent, i.e. it must explicitly be set to
	// false
	EnableSelfUpgrade = true
)

const (
	DBEnvVar       = "NITROUS_PG_URL"
	MixpanelEnvVar = "MIXPANEL_TOKEN"
)

func init() {
	AESKey = os.Getenv("AES_KEY")
	Debug = os.Getenv("DEBUG")
	DefaultAptDist = os.Getenv("DEFAULT_APT_DIST")
	DefaultAptLocation = os.Getenv("DEFAULT_APT_LOCATION")
	DefaultProvider = os.Getenv("DEFAULT_PROVIDER")
	DockerVersion = os.Getenv("DOCKER_VERSION")
	FreighterdVersion = os.Getenv("FREIGHTERD_VERSION")
	HostBuilderURL = os.Getenv("HOSTBUILDER_URL")
	HostHostname = os.Getenv("HOST_HOSTNAME")
	MixpanelToken = os.Getenv(MixpanelEnvVar)
	NitrousPgUrl = os.Getenv(DBEnvVar)
	ProxyHostname = os.Getenv("PROXY_HOSTNAME")
	PreviewHostname = os.Getenv("PREVIEW_HOSTNAME")
	WebHostname = os.Getenv("WEB_HOSTNAME")

	CAKeyPath = os.Getenv("CA_KEY_PATH")
	CACertPath = os.Getenv("CA_CERT_PATH")

	LibratoUser = os.Getenv("LIBRATO_USER")
	LibratoToken = os.Getenv("LIBRATO_TOKEN")
	LibratoInterval = os.Getenv("LIBRATO_INTERVAL")

	SendgridUsername = os.Getenv("SENDGRID_USERNAME")
	SendgridPassword = os.Getenv("SENDGRID_PASSWORD")

	StripePublicKey = os.Getenv("STRIPE_PUBLIC_KEY")
	StripeSecretKey = os.Getenv("STRIPE_SECRET_KEY")

	ProvisionerUser = os.Getenv("PROVISIONER_USER")
	ProvisionerInterval = os.Getenv("PROVISIONER_INTERVAL")

	ActivationEmailInterval = os.Getenv("ACTIVATION_EMAIL_INTERVAL")

	GetStartedEmailIntervalFirst = os.Getenv("GET_STARTED_INTERVAL_FIRST")
	GetStartedEmailIntervalSecond = os.Getenv("GET_STARTED_INTERVAL_SECOND")
	GetStartedEmailIntervalThird = os.Getenv("GET_STARTED_INTERVAL_THIRD")

	GitHubOAuthClientID = os.Getenv("GITHUB_CLIENT_ID")
	GitHubOAuthClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")

	enableUpgradeEnv := os.Getenv("ENABLE_SELF_UPGRADE")
	if enableUpgradeEnv == "" {
		EnableSelfUpgrade = true
	} else {
		var err error
		EnableSelfUpgrade, err = strconv.ParseBool(enableUpgradeEnv)
		if err != nil {
			log.Fatalf("ENABLE_SELF_UPGRADE not acceptable boolean: %s", err)
		}
	}
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
