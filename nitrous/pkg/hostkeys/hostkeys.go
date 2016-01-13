package hostkeys

import (
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/Sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/nitrous-common/cipher"
)

var (
	// specific only to keys and command-line utils, so not in envvars
	// consumers will need to check the values
	SSHKeysDir string
	AWSEnv     string
)

func init() {
	SSHKeysDir = os.Getenv("SSH_KEYS_DIR")
	AWSEnv = os.Getenv("AWS_ENV")
}

type SshHost struct {
	ID                    int64          `db:"id"`
	Slug                  string         `db:"slug"`
	State                 string         `db:"state"`
	TemplateName          string         `db:"pt_name"`
	RegionIdentifier      sql.NullString `db:"region_identifier"`
	IPAddress             sql.NullString `db:"ip_address"`
	EncryptedPrivateKey   string         `db:"encrypted_private_key"`
	EncryptedPrivateKeyIv string         `db:"encrypted_private_key_iv"`
	Provider              string         `db:"provider"`
}

// performs a query that populates SshHost fields
// be very wary of SQL injection (not to be run with user input)
func Query(extra string) string {
	return fmt.Sprintf(`SELECT hosts.id, hosts.slug, hosts.region_identifier, hosts.ip_address, hosts.state,
	pt.provider, pt.name AS pt_name,
	key_pairs.encrypted_private_key, key_pairs.encrypted_private_key_iv
	FROM hosts
	JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
	JOIN key_pairs ON key_pairs.host_id = hosts.id
	%s`, extra)
}

func (h *SshHost) PrivateKey() (*rsa.PrivateKey, error) {
	if h.useLocalKey() {
		privKey, err := h.amazonPrivateKey()
		if err == nil {
			return privKey, nil
		}

		logrus.Infof("failed to use local private key, using key from database, err: %v", err)
	}

	// get the private key from keypair
	priv, err := cipher.Decrypt(
		[]byte(envvars.AESKey),
		[]byte(h.EncryptedPrivateKeyIv),
		[]byte(h.EncryptedPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key for host %v, error %q", h, err)
	}

	pk, err := ssh.ParseRawPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key for host %v, error %q", h, err)
	}
	privKey, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid RSA key for host %v", h)
	}

	return privKey, nil
}

func (h *SshHost) PrivatePEM() ([]byte, error) {
	pk, err := h.PrivateKey()
	if err != nil {
		return nil, err
	}
	// ASN.1 DER encoded form
	privder := x509.MarshalPKCS1PrivateKey(pk)
	privblk := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privder,
	}
	return pem.EncodeToMemory(&privblk), nil
}

func FromSlug(d db.QueryReceiver, slug string) (*SshHost, error) {
	host := &SshHost{}
	err := d.Get(host,
		Query(`WHERE hosts.slug = $1 AND hosts.terminated_at IS NULL;`),
		slug)
	if err != nil {
		return nil, err
	}
	return host, nil
}

func (h *SshHost) amazonPrivateKey() (*rsa.PrivateKey, error) {
	keyPath := path.Join(SSHKeysDir, fmt.Sprintf(
		"hostbuilder.%s.%s.pem", h.RegionIdentifier.String, AWSEnv))
	if _, err := os.Stat(keyPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("private key does not exist locally at %q", keyPath)
		}
		return nil, fmt.Errorf("error stat-ing key at %q, err: %v", keyPath, err)
	}

	data, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("error reading private key at %q, err: %v", keyPath, err)
	}
	block, _ := pem.Decode(data)
	pk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse a valid RSA private key from %q, err: %v", keyPath, err)
	}

	logrus.Infof("using private key at %v", keyPath)

	return pk, nil
}

func (h *SshHost) Address() string {
	return fmt.Sprintf("%s:65100", h.IPAddress.String)
}

func (h *SshHost) useLocalKey() bool {
	// For the "amazon" provider, the private key is an EC2 keypair
	// We check if we have a private key for it in SSH_KEYS_DIR
	return h.Provider == "amazon" && SSHKeysDir != ""
}
