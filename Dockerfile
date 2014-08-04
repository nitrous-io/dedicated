From ubuntu:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  git-core \
  mercurial

RUN curl -s https://storage.googleapis.com/golang/go1.3.linux-amd64.tar.gz \
  | tar -v -C /usr/local -xz

ENV GOPATH /opt/src/github.com/nitrous-io/nitrous/.vendor:/opt
ENV PATH /usr/local/go/bin:/opt/bin:$PATH
ENV GOBIN /opt/bin

RUN mkdir -p /opt/src/github.com/nitrous-io/nitro
ADD . /opt/src/github.com/nitrous-io/nitro
ADD util/ssh_config /etc/ssh/ssh_config

WORKDIR /opt/src/github.com/nitrous-io/nitro
