package docker

const dockerfileUbuntuTmpl = `
FROM {{.Image}}
MAINTAINER Nitrous.IO, hello@nitrous.io

# compatible with images based on ubuntu-upstart

# Create action user and give sudo
RUN \
	useradd --create-home -s /bin/bash action && \
	adduser action sudo && \
	mkdir -p /etc/sudoers.d && \
	echo %action ALL=NOPASSWD:ALL > /etc/sudoers.d/action && \
	chmod 0440 /etc/sudoers.d/action

# Install essential tools
RUN \
	apt-get install -yq ssh sudo && \
	apt-get clean

# Enable system-level authorized ssh keys
RUN \
	sed -i '1s|^|# WARNING: EDITING THIS FILE MAY RENDER THIS CONTAINER INACCESSIBLE!!!\n\n|' /etc/ssh/sshd_config && \
	sed -i 's|PermitRootLogin without-password|PermitRootLogin no|g' /etc/ssh/sshd_config && \
	sed -i 's|#AuthorizedKeysFile|AuthorizedKeysFile /etc/ssh/freighter/authorized_keys|g' /etc/ssh/sshd_config && \
	sed -i 's|#PasswordAuthentication yes|PasswordAuthentication no|g' /etc/ssh/sshd_config && \
	mkdir /var/run/sshd

# Regenerate SSH host key && generate ssh keypair
RUN \
	rm -rf /etc/ssh/ssh_host_* && \
	ssh-keygen -A && \
	mkdir /home/action/.ssh && \
	chmod 0700 /home/action/.ssh && \
	ssh-keygen -b 2048 -t rsa -C 'action@{{.Name}}' -f /home/action/.ssh/id_rsa && \
	chown -R action:action /home/action/.ssh
`
