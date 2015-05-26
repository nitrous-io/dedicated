package envvars

import (
	"log"
	"os"
	"strconv"
	"strings"
)

const (
	DBEnvVar       = "NITROUS_PG_URL"
	MixpanelEnvVar = "MIXPANEL_TOKEN"
)

var (
	AESKey             = os.Getenv("AES_KEY")
	TrustKey           = os.Getenv("TRUST_KEY")
	Debug              = os.Getenv("DEBUG")
	DefaultAptDist     = os.Getenv("DEFAULT_APT_DIST")
	DefaultAptLocation = os.Getenv("DEFAULT_APT_LOCATION")
	DefaultProvider    = os.Getenv("DEFAULT_PROVIDER")
	DockerVersion      = os.Getenv("DOCKER_VERSION")
	FreighterdVersion  = os.Getenv("FREIGHTERD_VERSION")
	HostBuilderURL     = os.Getenv("HOSTBUILDER_URL")
	HostHostname       = os.Getenv("HOST_HOSTNAME")
	MixpanelToken      = os.Getenv(MixpanelEnvVar)
	NitrousPgUrl       = os.Getenv(DBEnvVar)
	ProxyHostname      = os.Getenv("PROXY_HOSTNAME")
	PreviewHostname    = os.Getenv("PREVIEW_HOSTNAME")
	WebHostname        = os.Getenv("WEB_HOSTNAME")

	StopTimeout = os.Getenv("STOP_TIMEOUT")
	KillTimeout = os.Getenv("KILL_TIMEOUT")

	CAKeyPath  = os.Getenv("CA_KEY_PATH")
	CACertPath = os.Getenv("CA_CERT_PATH")

	LibratoUser     = os.Getenv("LIBRATO_USER")
	LibratoToken    = os.Getenv("LIBRATO_TOKEN")
	LibratoInterval = os.Getenv("LIBRATO_INTERVAL")

	SendgridUsername = os.Getenv("SENDGRID_USERNAME")
	SendgridPassword = os.Getenv("SENDGRID_PASSWORD")

	StripePublicKey = os.Getenv("STRIPE_PUBLIC_KEY")
	StripeSecretKey = os.Getenv("STRIPE_SECRET_KEY")

	ProvisionerUser     = os.Getenv("PROVISIONER_USER")
	ProvisionerInterval = os.Getenv("PROVISIONER_INTERVAL")

	ActivationEmailInterval = os.Getenv("ACTIVATION_EMAIL_INTERVAL")

	GetStartedEmailIntervalFirst  = os.Getenv("GET_STARTED_INTERVAL_FIRST")
	GetStartedEmailIntervalSecond = os.Getenv("GET_STARTED_INTERVAL_SECOND")
	GetStartedEmailIntervalThird  = os.Getenv("GET_STARTED_INTERVAL_THIRD")

	GitHubOAuthClientID     = os.Getenv("GITHUB_CLIENT_ID")
	GitHubOAuthClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")

	TwilioNumber     = os.Getenv("TWILIO_NUMBER")
	TwilioAccountSID = os.Getenv("TWILIO_ACCOUNT_SID")
	TwilioAuthToken  = os.Getenv("TWILIO_AUTH_TOKEN")

	SecretToken = os.Getenv("SECRET_TOKEN")

	// Feature flag for queue
	EnableQueue = parseBoolWithDefault(os.Getenv("ENABLE_QUEUE"), false)

	RedisHost = os.Getenv("REDIS_HOST")
	RedisPort = os.Getenv("REDIS_PORT")
	RedisDB   = parseInt64(os.Getenv("REDIS_DB"))

	// Controls whether 'update now' option is available to users.  Defaults to
	// true if ENABLE_SELF_UPGRADE is absent, i.e. it must explicitly be set to
	// false

	EnableSelfUpgrade = parseBoolWithDefault(os.Getenv("ENABLE_SELF_UPGRADE"), true)

	// email blacklist
	EmailBlacklistPath = os.Getenv("EMAIL_BLACKLIST_PATH")
)

func parseBoolWithDefault(s string, def bool) bool {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return def
	}
	return b
}

func parseInt64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}

func CheckCA() {
	if CAKeyPath == "" {
		log.Fatal("CA_KEY_PATH is not set")
	}
	if CACertPath == "" {
		log.Fatal("CA_CERT_PATH is not set")
	}
}

func CheckDB() {
	if NitrousPgUrl == "" {
		log.Fatalf("nitrousd: %s is not defined", DBEnvVar)
	}
}

func CheckSendgrid() {
	if SendgridUsername == "" {
		log.Fatal("SENDGRID_USERNAME not set")
	} else if strings.HasPrefix(SendgridUsername, "disable") {
		log.Println("Warning: Sendgrid mailer disabled.")
		return
	}
	if SendgridPassword == "" {
		log.Fatal("SENDGRID_PASSWORD not set")
	}
}

func CheckStripe() {
	if StripeSecretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY environment variable not set")
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

func CheckSecretToken() {
	if SecretToken == "" {
		log.Fatal("SECRET_TOKEN not set")
	}
}
