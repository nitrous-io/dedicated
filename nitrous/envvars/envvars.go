package envvars

import (
	"net/mail"
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
	HostBuilderURL     = os.Getenv("HOSTBUILDER_URL")
	HostHostname       = os.Getenv("HOST_HOSTNAME")
	MixpanelToken      = os.Getenv(MixpanelEnvVar)
	NitrousPgUrl       = os.Getenv(DBEnvVar)
	ProxyHostname      = os.Getenv("PROXY_HOSTNAME")
	PreviewHostname    = os.Getenv("PREVIEW_HOSTNAME")
	WebHostname        = os.Getenv("WEB_HOSTNAME")

	DockerRegistryHostname     = os.Getenv("DOCKER_REGISTRY_HOSTNAME")
	DockerRegistryWebhookToken = os.Getenv("DOCKER_REGISTRY_WEBHOOK_TOKEN")

	BetaDashboardLocation = os.Getenv("BETA_DASHBOARD_LOCATION")
	BetaEarnBonusLocation = os.Getenv("BETA_EARN_BONUS_LOCATION")
	BetaLoginLocation     = os.Getenv("BETA_LOGIN_LOCATION")
	BetaSignupLocation    = os.Getenv("BETA_SIGNUP_LOCATION")

	StopTimeout = os.Getenv("STOP_TIMEOUT")
	KillTimeout = os.Getenv("KILL_TIMEOUT")

	CAKeyPath  = os.Getenv("CA_KEY_PATH")
	CACertPath = os.Getenv("CA_CERT_PATH")

	LibratoUser     = os.Getenv("LIBRATO_USER")
	LibratoToken    = os.Getenv("LIBRATO_TOKEN")
	LibratoInterval = os.Getenv("LIBRATO_INTERVAL")

	SendgridUsername = os.Getenv("SENDGRID_USERNAME")
	SendgridPassword = os.Getenv("SENDGRID_PASSWORD")

	HelpScoutAPIKey      = os.Getenv("HELPSCOUT_API_KEY")
	HelpScoutMailboxName = os.Getenv("HELPSCOUT_MAILBOX_NAME")
	HelpScoutMailboxID   = parseInt64(os.Getenv("HELPSCOUT_MAILBOX_ID"))
	HelpScoutUserID      = parseInt64(os.Getenv("HELPSCOUT_USER_ID"))

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

	TwitterOAuthConsumerKey    = os.Getenv("TWITTER_CONSUMER_KEY")
	TwitterOAuthConsumerSecret = os.Getenv("TWITTER_CONSUMER_SECRET")

	SecretToken = os.Getenv("SECRET_TOKEN")

	QueueType    = os.Getenv("QUEUE_TYPE")
	RabbitMQAddr = os.Getenv("RABBITMQ_ADDR")

	RedisHost = os.Getenv("REDIS_HOST")
	RedisPort = os.Getenv("REDIS_PORT")
	RedisDB   = parseInt64(os.Getenv("REDIS_DB"))

	// blacklists
	EmailBlacklistPath = os.Getenv("EMAIL_BLACKLIST_PATH")
	CIDRBlacklistPath  = os.Getenv("CIDR_BLACKLIST_PATH")

	// Queue handler timeout
	QueueHandlerTimeout = parseDurationWithDefault(os.Getenv("QUEUE_HANDLER_TIMEOUT"), 5*time.Minute)

	// Defer Panic Token
	DeferPanicToken = os.Getenv("DEFER_PANIC_TOKEN")

	// Folder for cpu profiling
	ProfileDumpFolder = os.Getenv("PROFILE_DUMP_FOLDER")

	SupportEmail     = os.Getenv("SUPPORT_EMAIL_ADDRESS")
	SupportEmailAddr *mail.Address

	MailChimpAPIUrl          = os.Getenv("MAILCHIMP_API_URL")
	MailChimpAPIKey          = os.Getenv("MAILCHIMP_API_KEY")
	MailChimpIDNewsletter    = os.Getenv("MAILCHIMP_ID_NEWSLETTER")
	MailChimpIDAnnouncements = os.Getenv("MAILCHIMP_ID_ANNOUNCEMENTS")

	// Google ReCaptcha
	CaptchaPublicKey = os.Getenv("GOOGLE_RECAPTCHA_PUBLIC_KEY")
	CaptchaSecretKey = os.Getenv("GOOGLE_RECAPTCHA_SECRET_KEY")

	GithubAccessToken = os.Getenv("GITHUB_ACCESS_TOKEN")
)

func init() {
	if strings.HasSuffix(os.Args[0], ".test") && !Testing {
		logrus.Fatal("It appears that you are running tests, but the env var \"TESTING\" is not set!")
	}

	if SupportEmail == "" {
		SupportEmail = `"Nitrous" <support@nitrous.io>`
	}
	var err error
	SupportEmailAddr, err = mail.ParseAddress(SupportEmail)
	if err != nil {
		logrus.Fatalf("Error parsing e-mail address %s: %v", SupportEmail, err)
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

func CheckHelpScout() {
	if HelpScoutAPIKey == "" || HelpScoutAPIKey == "disable" {
		logrus.Warn("HELPSCOUT_API_KEY not set")
	}
	if HelpScoutMailboxName == "" || HelpScoutMailboxName == "disable" {
		logrus.Warn("HELPSCOUT_MAILBOX_NAME not set")
	}
	if HelpScoutMailboxID == 0 {
		logrus.Warn("HELPSCOUT_MAILBOX_ID not set")
	}
	if HelpScoutUserID == 0 {
		logrus.Warn("HELPSCOUT_USER_ID not set")
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

func IsCaptchaAvailable() bool {
	return CaptchaPublicKey != "" && CaptchaSecretKey != ""
}
