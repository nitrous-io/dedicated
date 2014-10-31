# -*- mode: ruby -*-
# vi: set ft=ruby :

VAGRANTFILE_API_VERSION = '2'

## NOTE: how the paths are set up
# dedicated repo root is synced to $GOPATH/src/github.com/nitrous-io/dedicated
# , which is symlinked to /home/vagrant/workspace
# This means that all changes made when working from repo root on your host
# will automatically be reflected in builds/tests/etc.

$script = <<SCRIPT

export VAGRANT_HOME=/home/vagrant
export GOPATH=$VAGRANT_HOME/go
export GOROOT=/usr/local/opt/go
export WORKSPACE=$VAGRANT_HOME/workspace
export RUBYBIN=/usr/local/ruby/2.1.2/bin
export PATH=$GOPATH/bin:$GOROOT/bin:$RUBYBIN:$PATH

## BASIC APT DEPENDENCIES ####################################################

echo "Installing basic dependencies..."

apt-get update >/dev/null
apt-get install -yy --only-upgrade libcgmanager0
apt-get install -yy --no-install-recommends build-essential curl git-core mercurial dnsmasq

## CONFIGURE POSTGRESQL ######################################################

echo "Installing postgres..."

apt-get install -yy postgresql-9.3 postgresql-contrib-9.3 libpq-dev

if ! sudo su postgres -c 'psql -c "SELECT usename FROM pg_user"' | grep vagrant > /dev/null; then
  sudo su postgres -c 'psql -c "CREATE ROLE vagrant NOSUPERUSER CREATEDB NOCREATEROLE INHERIT LOGIN;"'
fi

## CONFIGURE MERCURIAL #######################################################

echo "Configuring mercurial..."

cat > $HOME/.hgrc <<EOF
[trusted]
users = vagrant
groups = vagrant
EOF

## INSTALL GO ################################################################

if ! go version | grep go1.3.3 > /dev/null; then
  echo "Installing go..."

  mkdir -p $GOROOT
  curl -s https://storage.googleapis.com/golang/go1.3.3.linux-amd64.tar.gz| tar -C /usr/local/opt -xz
fi

## INSTALL TOOLS #############################################################

echo "Installing tools..."

if [[ ! -d $GOPATH/src/code.google.com/p/go.tools/cmd/goimports ]]; then
  go get code.google.com/p/go.tools/cmd/goimports
fi
if [[ ! -d $GOPATH/src/github.com/tools/godep ]]; then
  go get github.com/tools/godep
fi
if [[ ! -d $GOPATH/src/github.com/ddollar/forego ]]; then
  go get github.com/ddollar/forego
fi

## INSTALL DOCKER ############################################################

if [[ ! -x /usr/bin/docker ]]; then
  echo "Installing docker..."

  apt-get install -y apt-transport-https
  echo deb https://get.docker.io/ubuntu docker main > /etc/apt/sources.list.d/docker.list
  apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9
  apt-get update
  apt-get install -yy lxc-docker-1.3.0
fi

cat > /etc/default/docker <<EOF
  DOCKER_OPTS="-H tcp://0.0.0.0:2375 -H unix:///var/run/docker.sock"
EOF

service docker restart

adduser vagrant docker

docker pull nitrousio/ubuntu-dind >/dev/null 2>&1 &
sudo chown -R vagrant:vagrant /var/lib/docker

## INSTALL RUBY ##############################################################

if [[ ! -x $RUBYBIN/ruby ]]; then
  echo "Installing ruby..."

  mkdir -p /usr/local/ruby
  curl -s https://intercityup.com/binaries/ruby/ubuntu/14.04/x86_64/ruby-2.1.2.tar.bz2 \
    | tar -C /usr/local/ruby -xj
fi

sudo -u vagrant bash <<EOF
  mkdir -p $VAGRANT_HOME/.bundle
  echo "BUNDLE_PATH: .bundle" > $VAGRANT_HOME/.bundle/config
  echo "BUNDLE_BIN: .bundle/binstubs" >> $VAGRANT_HOME/.bundle/config
EOF

$RUBYBIN/gem install bundler --no-ri --no-rdoc

## INSTALL NODE ##############################################################

echo "Installing node..."

sudo apt-get install -yy --no-install-recommends nodejs

## INSTALL COMPONENTS ########################################################

echo "Installing components..."

upstart_component() {
  component=$1
  cat >/etc/init/$component.conf <<EOF

    start on vagrant-mounted
    stop on runlevel [06]

    setuid vagrant
    setgid vagrant

    env GOPATH=$GOPATH
    env GOROOT=$GOROOT
    env PATH=/home/vagrant/.bundle/binstubs:$PATH

    exec /bin/bash -c "cd /home/vagrant/workspace/dedicated && make run-$component"

    respawn
    respawn limit 3 60

EOF
}

## PATH SETUP ################################################################

mkdir -p $WORKSPACE
mkdir -p $GOPATH/src/github.com/nitrous-io
DEDIREPO=$GOPATH/src/github.com/nitrous-io/dedicated
ln -nfs $DEDIREPO $WORKSPACE/dedicated

sed -i '/## DEV PATHS BEGIN/,/## DEV PATHS END/d' /etc/profile
cat >> /etc/profile << EOF
## DEV PATHS BEGIN
export GOPATH=$DEDIREPO/Godeps/_workspace:$GOPATH
export GOROOT=$GOROOT
export PATH=$DEDIREPO/Godeps/_workspace/bin:$GOPATH/bin:$GOROOT/bin:$RUBYBIN:\\$PATH
## DEV PATHS END
EOF

sudo chown -R vagrant:vagrant $GOPATH


## COMPONENT SETUP ###########################################################

upstart_component nitrous
upstart_component hostbuilder

sed -i 's/make run-nitrous\"/make run-nitrous-dev\"/' /etc/init/nitrous.conf

sudo -u vagrant bash <<EOF
  source /etc/profile

  if [[ ! -f $WORKSPACE/dedicated/hostbuilder/config.json ]]; then
    cp $WORKSPACE/dedicated/hostbuilder/{sample_,}config.json
  fi

  if [[ ! -f $WORKSPACE/dedicated/nitrous/.env ]]; then
    cp $WORKSPACE/dedicated/nitrous/.env{-example,}
  fi

  # build ginkgo so it's available for tests w/o having to build first
  #cd $DEDIREPO/Godeps/_workspace/src/github.com/onsi/ginkgo && godep go install ./...

  if ! sudo su postgres -c 'psql -d nitrous_development -c "SELECT 1"'; then
    cd $DEDIREPO && make createdb
  fi

  if ! psql nitrous_development -c 'SELECT email FROM users' | grep admins@nitrous.io; then
    cd $DEDIREPO && make nitrous && NITROUS_PG_URL="host=/var/run/postgresql user=vagrant dbname=nitrous_development sslmode=disable" build/dbseed
  fi
EOF


## CONFIGURE NGINX ###########################################################

apt-get install -yy nginx

cat > /etc/nginx/nginx.conf <<EOF
user www-data;
worker_processes 6;
pid /run/nginx.pid;

events {
  worker_connections 768;
  # multi_accept on;
}

http {
  include /etc/nginx/mime.types;
  default_type application/octet-stream;

  access_log /var/log/nginx/access.log;
  error_log /var/log/nginx/error.log;

  sendfile on;

  map \\$http_upgrade \\$connection_upgrade {
    default upgrade;
    ''      close;
  }

  server {
    listen 8080 ssl;

    server_name pro.n2odev.com;
    ssl_certificate /home/vagrant/workspace/dedicated/nitrous/dev/n2odev.crt;
    ssl_certificate_key /home/vagrant/workspace/dedicated/nitrous/dev/n2odev.key;

    keepalive_timeout 70;

    location / {
      proxy_pass http://127.0.0.1:3000;
    }

    location /api/v0/ {
      proxy_pass http://127.0.0.1:3333;
      proxy_http_version 1.1;
      proxy_set_header Upgrade \\$http_upgrade;
      proxy_set_header Connection \\$connection_upgrade;
    }
  }
}
EOF

## LAUNCH SERVICES ###########################################################

service hostbuilder stop || true; service hostbuilder start
service nitrous stop || true; service nitrous start
service nginx restart

SCRIPT

# allow devs to overload config
additional_config = lambda do |config|
  (49000..49999).each do |port|
    config.vm.network "forwarded_port", guest: port, host: port
  end
end

custom_file = File.join(File.dirname(__FILE__), "custom_config.rb")
if File.exists? custom_file
  eval_str = "additional_config = lambda do |config| #{File.read("custom_config.rb")} end"
  eval eval_str
end

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|
  config.vm.hostname = 'dedicated-vm'
  config.vm.box = 'nitrous-io/trusty64'

  config.vm.provider :virtualbox do |v|
    v.memory = ENV['DEDICATED_VM_MEM'] || 2048
    v.cpus   = ENV['DEDICATED_VM_CPU'] || 2
    v.customize ['modifyvm', :id, '--natdnshostresolver1', 'on']
    v.customize ['modifyvm', :id, '--natdnsproxy1', 'on']
  end

  config.vm.provider 'vmware_fusion' do |v|
    v.gui = true
    v.vmx['memsize'] = ENV['DEDICATED_VM_MEM'] || '2048'
    v.vmx['numvcpus'] = ENV['DEDICATED_VM_CPU'] || '2'
  end

  if private_ip = ENV['DEDICATED_VM_INTERFACE']
    config.vm.network :private_network, ip: private_ip
  end

  config.vm.network "forwarded_port", guest: 8080,  host: 8080
  additional_config.call config

  config.ssh.shell = "bash -c 'BASH_ENV=/etc/profile exec bash'" # avoids 'stdin: is not a tty' error.
  config.ssh.forward_agent = true
  config.vm.provision 'shell', inline: $script

  config.vm.synced_folder ".", "/home/vagrant/go/src/github.com/nitrous-io/dedicated"
end
