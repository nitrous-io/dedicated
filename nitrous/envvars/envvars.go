package envvars

import (
	"os"
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
	NoSSLVerify        string
	ProxyHostname      string
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
}
