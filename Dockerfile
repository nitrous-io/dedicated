FROM nitrousio/goboxer-ci
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  git \
  libsqlite3-dev \
  mercurial \
  ruby1.9.1 \
  sqlite3 \
  sudo

RUN curl -s https://storage.googleapis.com/golang/go1.2.2.linux-amd64.tar.gz \
  | tar -C /usr/local -xz

RUN mkdir -p /opt/src/github.com/nitrous-io/freighter
RUN mkdir -p /root/.ssh && ssh-keygen -t rsa -N "" -f /root/.ssh/id_rsa

ENV PATH /usr/local/go/bin:$PATH
ENV GOPATH /opt/src/github.com/nitrous-io/freighter/_vendor:/opt
ENV HOME /root

WORKDIR /opt/src/github.com/nitrous-io/freighter
ADD . /opt/src/github.com/nitrous-io/freighter
