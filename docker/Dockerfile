FROM nitrousio/ubuntu-dind:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y --no-install-recommends \
  build-essential \
  curl \
  git \
  libsqlite3-dev \
  mercurial \
  ruby1.9.1 \
  ruby1.9.1-dev \
  sqlite3 \
  lxc \
  sudo \
  tmux

RUN curl -s https://storage.googleapis.com/golang/go1.2.2.linux-amd64.tar.gz \
  | tar -C /usr/local -xz

RUN gem install fpm --no-rdoc --no-ri --version 1.0.2

RUN mkdir -p /opt/src/github.com/nitrous-io/freighter
RUN mkdir -p /root/.ssh && ssh-keygen -t rsa -N "" -f /root/.ssh/id_rsa

ENV VENDOR_PATH /opt/src/github.com/nitrous-io/freighter/_vendor
ENV GOPATH $VENDOR_PATH:/opt
ENV GOBIN $VENDOR_PATH/bin
ENV PATH $GOBIN:/usr/local/go/bin:$PATH
ENV HOME /root
ENV WORKDIR /opt/src/github.com/nitrous-io/freighter

WORKDIR /opt/src/github.com/nitrous-io/freighter
