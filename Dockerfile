FROM ubuntu:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y \
  curl \
  git \
  libpq-dev \
  mercurial \
  postgresql-9.3 \
  postgresql-contrib-9.3
  
RUN curl -s https://storage.googleapis.com/golang/go1.3.linux-amd64.tar.gz | tar -v -C /usr/local -xz

ENV GOPATH /opt/src/github.com/nitrous-io/tender/.vendor:/opt
ENV PATH /usr/local/go/bin:/opt/bin:$PATH

RUN mkdir -p /opt/src/github.com/nitrous-io/nitrous
WORKDIR /opt/src/github.com/nitrous-io/

RUN locale-gen en_US.utf8
ADD ./script/createdb /createdb
RUN /etc/init.d/postgresql start && /createdb
