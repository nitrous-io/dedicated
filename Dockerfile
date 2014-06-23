From ubuntu:12.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  git \
  mercurial \
  ruby1.9.3

RUN curl -s https://storage.googleapis.com/golang/go1.2.2.linux-amd64.tar.gz \
  | tar -v -C /usr/local -xz

RUN mkdir -p /opt/src/github.com/nitrous-io/hostbuilder

ENV PATH /usr/local/go/bin:$PATH
ENV GOPATH /opt/src/github.com/nitrous-io/hostbuilder/.vendor:/opt

WORKDIR /opt/src/github.com/nitrous-io/hostbuilder
ADD . /opt/src/github.com/nitrous-io/hostbuilder
