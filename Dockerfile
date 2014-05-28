FROM nitrousio/ubuntu-dind:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  git \
  libsqlite3-dev \
  mercurial \
  ruby1.9.1 \
  sqlite3 \
  lxc \
  sudo

RUN curl -s https://storage.googleapis.com/golang/go1.2.2.linux-amd64.tar.gz \
  | tar -C /usr/local -xz

RUN mkdir -p /opt/src/github.com/nitrous-io/freighter
RUN mkdir -p /root/.ssh && ssh-keygen -t rsa -N "" -f /root/.ssh/id_rsa

ENV VENDOR_PATH /opt/src/github.com/nitrous-io/freighter/_vendor
ENV GOPATH $VENDOR_PATH:/opt
ENV GOBIN $VENDOR_PATH/bin
ENV PATH $GOBIN:/usr/local/go/bin:$PATH
ENV HOME /root

WORKDIR /opt/src/github.com/nitrous-io/freighter
