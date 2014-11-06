package host

import (
	"io"

	"github.com/nitrous-io/dedicated/pkg/sshutil"
)

var (
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"
)

type logger interface {
	Logf(format string, v ...interface{})
}

func WriteFile(l logger, c *sshutil.Conn, files map[string]io.Reader) error {
	for dest, val := range files {
		if err := c.Upload(dest, val); err != nil {
			l.Logf("Failed to upload %s", dest)
			return err
		}
		l.Logf("Uploaded %s", dest)
	}
	return nil
}

func CreateEtcFreighterd(c *sshutil.Conn) error {
	return c.Execute("mkdir -p /etc/freighterd")
}

func UpdateSoftware(l logger, c *sshutil.Conn) error {
	l.Logf("Updating freighterd package")
	cmd :=
		`DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" freighterd`
	if err := c.Execute(cmd); err != nil {
		return err
	}
	l.Logf("Updated freighterd package")
	return nil
}

func RestartFreighterd(l logger, c *sshutil.Conn) error {
	l.Logf("Restarting freighterd")
	if err := c.Execute(`service freighterd restart`); err != nil {
		return err
	}
	l.Logf("Restarted freighterd")
	return nil
}
