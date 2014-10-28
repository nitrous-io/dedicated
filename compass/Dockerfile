From ubuntu:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  git \
  mercurial \
  postgresql \
  sudo

RUN /etc/init.d/postgresql start \
  && sudo -u postgres createuser -U postgres --superuser root \
  && sudo -u postgres createdb -U postgres -O root -E utf8 testdb -T template0

RUN curl -s https://storage.googleapis.com/golang/go1.3.linux-amd64.tar.gz \
  | tar -v -C /usr/local -xz

ENV PATH /opt/bin:/usr/local/go/bin:$PATH
RUN GOPATH=/opt go get github.com/nitrous-io/goop

RUN mkdir -p /opt/src/github.com/nitrous-io/compass
ENV GOPATH /opt/src/github.com/nitrous-io/compass/.vendor:/opt

# build the latest source with its dependencies
WORKDIR /opt/src/github.com/nitrous-io/compass
# if dependencies doesnt change, dont install extra
ADD ./Goopfile /opt/src/github.com/nitrous-io/compass/Goopfile
ADD ./Goopfile.lock /opt/src/github.com/nitrous-io/compass/Goopfile.lock
RUN goop install

ADD . /opt/src/github.com/nitrous-io/compass
