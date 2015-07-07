package envvars

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/kardianos/osext"
	"github.com/nitrous-io/nitrous-common/must"
)

const (
	DBEnvVar       = "NITROUS_PG_URL"
	MixpanelEnvVar = "MIXPANEL_TOKEN"
)

var (
	Testing = parseBoolWithDefault(os.Getenv("TESTING"), false)

	RootDir = must.String(osext.ExecutableFolder())

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

	RedisHost = os.Getenv("REDIS_HOST")
	RedisPort = os.Getenv("REDIS_PORT")
	RedisDB   = parseInt64(os.Getenv("REDIS_DB"))

	// Controls whether 'update now' option is available to users.  Defaults to
	// true if ENABLE_SELF_UPGRADE is absent, i.e. it must explicitly be set to
	// false

	EnableSelfUpgrade = parseBoolWithDefault(os.Getenv("ENABLE_SELF_UPGRADE"), true)

	// blacklists
	EmailBlacklistPath = os.Getenv("EMAIL_BLACKLIST_PATH")
	CIDRBlacklistPath  = os.Getenv("CIDR_BLACKLIST_PATH")

	// Queue handler timeout
	QueueHandlerTimeout = parseDurationWithDefault(os.Getenv("QUEUE_HANDLER_TIMEOUT"), 5*time.Minute)

	// Defer Panic Token
	DeferPanicToken = os.Getenv("DEFER_PANIC_TOKEN")
)

func init() {
	if strings.HasSuffix(os.Args[0], ".test") && !Testing {
		fmt.Println("ERROR: It appears that you are running tests, but the env var \"TESTING\" is not set!")
		os.Exit(1)
	}
}

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

func parseDurationWithDefault(s string, def time.Duration) time.Duration {
	var t time.Duration
	t, err := time.ParseDuration(os.Getenv("QUEUE_HANDLER_TIMEOUT"))
	if err != nil {
		return def
	}

	return t
}

func CheckCA() {
	if CAKeyPath == "" {
		logrus.Fatal("CA_KEY_PATH is not set")
	}
	if CACertPath == "" {
		logrus.Fatal("CA_CERT_PATH is not set")
	}
}

func CheckDB() {
	if NitrousPgUrl == "" {
		logrus.Fatalf("nitrousd: %s is not defined", DBEnvVar)
	}
}

func CheckSendgrid() {
	if SendgridUsername == "" {
		logrus.Fatal("SENDGRID_USERNAME not set")
	} else if strings.HasPrefix(SendgridUsername, "disable") {
		logrus.Warn("Sendgrid mailer disabled.")
		return
	}
	if SendgridPassword == "" {
		logrus.Fatal("SENDGRID_PASSWORD not set")
	}
}

func CheckStripe() {
	if StripeSecretKey == "" {
		logrus.Fatal("STRIPE_SECRET_KEY environment variable not set")
	}
}

func CheckHostnames() {
	if HostHostname == "" {
		logrus.Fatal("HOST_HOSTNAME not set")
	}
	if ProxyHostname == "" {
		logrus.Fatal("PROXY_HOSTNAME not set")
	}
	if PreviewHostname == "" {
		logrus.Fatal("PREVIEW_HOSTNAME not set")
	}
}

func CheckGitHubOAuth() {
	if GitHubOAuthClientID == "" {
		logrus.Warn("GITHUB_CLIENT_ID not set")
	}

	if GitHubOAuthClientSecret == "" {
		logrus.Warn("GITHUB_CLIENT_SECRET not set")
	}
}

func CheckSecretToken() {
	if SecretToken == "" {
		logrus.Fatal("SECRET_TOKEN not set")
	}
}

func CheckRedis() {
	if RedisHost == "" {
		logrus.Fatalf("nitrousd: REDIS_HOST is not set")
	}

	if RedisPort == "" {
		logrus.Fatalf("nitrousd: REDIS_PORT is not set")
	}
}
