package docker

import "text/template"

const dockerfileTmpl = `
FROM {{.Image}}

# compatible with images based on ubuntu-upstart

# Add public keys
RUN \
  sed -i 's|#AuthorizedKeysFile|AuthorizedKeysFile /etc/ssh/authorized_keys |g' /etc/ssh/sshd_config && \
  sed -i 's|#PasswordAuthentication yes|PasswordAuthentication no|g' /etc/ssh/sshd_config && \
  touch /etc/ssh/authorized_keys && \
  {{ range $i, $key := .PublicKeys }}echo '{{$key}}' >> /etc/ssh/authorized_keys && {{end}} \
  mkdir /var/run/sshd
`

var dockerfileTemplate *template.Template

func init() {
	dockerfileTemplate =
		template.Must(template.New("dockerfile").Parse(dockerfileTmpl))
}
