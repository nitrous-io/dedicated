From ubuntu:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y --no-install-recommends \
  build-essential \
  curl \
  git \
  mercurial \
  ruby1.9.3

RUN curl -s https://storage.googleapis.com/golang/go1.3.linux-amd64.tar.gz \
  | tar -v -C /usr/local -xz

RUN mkdir -p /opt/src/github.com/nitrous-io/hostbuilder

ENV GOPATH /opt
ENV PATH /usr/local/go/bin:/opt/bin:$PATH

ADD util/ssh_config /etc/ssh/ssh_config

WORKDIR /opt/src/github.com/nitrous-io/hostbuilder
ADD . /opt/src/github.com/nitrous-io/hostbuilder
