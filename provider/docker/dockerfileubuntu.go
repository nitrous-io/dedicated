package docker

const dockerfileUbuntuTmpl = `
FROM {{.Image}}

# compatible with images based on ubuntu-upstart

# create action user
RUN \
  useradd --create-home -s /bin/bash action && \
  adduser action sudo && \
  mkdir -p /etc/sudoers.d && \
  echo %action ALL=NOPASSWD:ALL > /etc/sudoers.d/action && \
  chmod 0440 /etc/sudoers.d/action && \
  echo 'action:action' | chpasswd

# Update things and make sure the required packges are installed
RUN \
  apt-get update && \
  apt-get install -yq ssh sudo && \
  apt-get clean

# Allow system level ssh keys
RUN \
  sed -i 's|#AuthorizedKeysFile|AuthorizedKeysFile /etc/ssh/authorized_keys |g' /etc/ssh/sshd_config && \
  sed -i 's|#PasswordAuthentication yes|PasswordAuthentication no|g' /etc/ssh/sshd_config && \
	touch /etc/ssh/authorized_keys && \
  {{ range $i, $key := .PublicKeys }}echo '{{$key}}' >> /etc/ssh/authorized_keys && {{end}} \
  mkdir /var/run/sshd
`
