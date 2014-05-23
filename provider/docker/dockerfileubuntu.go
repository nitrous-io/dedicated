package docker

const dockerfileUbuntuTmpl = `
FROM {{.Image}}

# compatible with images based on ubuntu-upstart

# Create action user and give sudo
RUN \
  useradd --create-home -s /bin/bash action && \
  adduser action sudo && \
  mkdir -p /etc/sudoers.d && \
  echo %action ALL=NOPASSWD:ALL > /etc/sudoers.d/action && \
  chmod 0440 /etc/sudoers.d/action

# Update and install essential tools
RUN \
  apt-get update && \
  apt-get install -yq ssh sudo && \
  apt-get clean

# Add public keys
RUN \
  sed -i 's|#AuthorizedKeysFile|AuthorizedKeysFile /etc/ssh/authorized_keys |g' /etc/ssh/sshd_config && \
  sed -i 's|#PasswordAuthentication yes|PasswordAuthentication no|g' /etc/ssh/sshd_config && \
  touch /etc/ssh/authorized_keys && \
  {{ range $i, $key := .PublicKeys }}echo '{{$key}}' >> /etc/ssh/authorized_keys && {{end}} \
  mkdir /var/run/sshd

# Generate ssh keypair
RUN \
  mkdir /home/action/.ssh && \
  chmod 0700 /home/action/.ssh && \
  ssh-keygen -b 2048 -t rsa -C 'action@{{.Name}}' -f /home/action/.ssh/id_rsa && \
  chown -R action:action /home/action/.ssh
`
