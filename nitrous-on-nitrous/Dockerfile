FROM quay.io/nitrous/docker:latest
MAINTAINER Nitrous, Inc. <eng@nitrous.io>

# set up postgres
RUN \
  apt-get update && \
  apt-get install -y \
    postgresql=9.3+154 \
    postgresql-contrib=9.3+154 \
    libpq-dev=9.3.6-0ubuntu0.14.04 && \
  apt-get clean

COPY support/pg_hba.conf /etc/postgresql/9.3/main/pg_hba.conf

RUN service postgresql start && \
  su postgres -c 'createuser --superuser $NITROUS_USERNAME' && \
  service postgresql stop

# configure mercurial
COPY support/hgrc /home/$NITROUS_USERNAME/.hgrc
RUN chown $NITROUS_USERNAME:$NITROUS_USERNAME /home/$NITROUS_USERNAME/.hgrc

# configure go
USER $NITROUS_USERNAME

RUN \
  /bin/bash -l -c \
    'go get github.com/onsi/ginkgo/ginkgo && \
    go get github.com/onsi/gomega && \
    go get github.com/nitrous-io/goop && \
    go get github.com/ddollar/forego'

USER root

# install ruby
RUN \
  mkdir -p /usr/local/opt/rbenv && \
  /bin/bash -c 'chown -R $NITROUS_USERNAME:$NITROUS_USERNAME /usr/local/opt/rbenv'

RUN \
  echo 'export RBENV_ROOT=/usr/local/opt/rbenv' >> /etc/profile && \
  echo 'export PATH=$RBENV_ROOT/bin:$PATH' >> /etc/profile && \
  echo 'eval "$($RBENV_ROOT/bin/rbenv init -)"' >> /etc/profile && \
  echo 'export PATH=.bundle/binstubs:$PATH' >> /etc/profile

COPY support/gemrc /home/$NITROUS_USERNAME/.gemrc
COPY support/bundle-config /home/$NITROUS_USERNAME/.bundle/config

RUN \
  /bin/bash -c 'chown -R $NITROUS_USERNAME:$NITROUS_USERNAME /home/$NITROUS_USERNAME/.gemrc /home/$NITROUS_USERNAME/.bundle'

USER $NITROUS_USERNAME

RUN \
  /bin/bash -c \
    'git clone https://github.com/sstephenson/rbenv.git /usr/local/opt/rbenv && \
    git clone https://github.com/sstephenson/ruby-build.git /usr/local/opt/rbenv/plugins/ruby-build && \
    git clone https://github.com/nitrous-io/rvm-download.git /usr/local/opt/rbenv/plugins/rvm-download'

RUN \
  /bin/bash -l -c \
    'rbenv download 2.1.5 && \
    rbenv global 2.1.5 && \
    rbenv rehash && \
    ruby -v && \
    gem install bundler --version=1.7.12 && \
    rbenv rehash'

USER root

# install nginx

RUN \
  echo "deb http://ppa.launchpad.net/nginx/stable/ubuntu $(lsb_release -sc) main" | tee /etc/apt/sources.list.d/nginx-stable.list && \
  apt-key adv --keyserver keyserver.ubuntu.com --recv-keys C300EE8C && \
  apt-get update && \
  DEBIAN_FRONTEND=noninteractive apt-get -y install \
    nginx=1.8.0-1+trusty1 && \
  apt-get clean

COPY support/nginx.conf /etc/nginx/nginx.conf
COPY support/nginx-default /etc/nginx/sites-available/default
COPY support/n2odev.crt /etc/nginx/certs/n2odev.crt
COPY support/n2odev.key /etc/nginx/certs/n2odev.key

# install dnsmasq
RUN \
  apt-get update && \
  apt-get install -y dnsmasq && \
  apt-get clean

# install required docker version
RUN \
  apt-get update && \
  apt-get install -y lxc-docker-1.6.0 && \
  apt-get clean

# copy nitrous enhanced docker
COPY support/docker-1.6.0-nitrous1 /usr/bin/docker

# copy setup.sh

COPY support/setup.sh /home/$NITROUS_USERNAME/setup.sh
RUN chown $NITROUS_USERNAME:$NITROUS_USERNAME /home/$NITROUS_USERNAME/setup.sh

USER $NITROUS_USERNAME
RUN ln -s /home/$NITROUS_USERNAME/code /home/$NITROUS_USERNAME/workspace
USER root

ENV NITROUS_README_URL https://codepen.io/raingrove/full/WbyvaZ/
