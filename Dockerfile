From nitrousio/ubuntu-dind:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y --no-install-recommends \
  build-essential \
  curl \
  git-core \
  mercurial \
  postgresql-9.3 \
  postgresql-contrib-9.3 \
  libpq-dev \
  nodejs

RUN locale-gen en_US.utf8

# set paths
ENV HOME /root
ENV GOBIN /opt/bin
ENV PATH /opt/bundle/binstubs:/usr/local/ruby/2.1.2/bin:/usr/local/go/bin:/opt/bin:$PATH
WORKDIR /opt/src/github.com/nitrous-io/dedicated

# install ruby
RUN mkdir -p /usr/local/ruby
RUN curl -s https://intercityup.com/binaries/ruby/ubuntu/14.04/x86_64/ruby-2.1.2.tar.bz2 \
  | tar -v -C /usr/local/ruby -xj

# install go
RUN curl -s https://storage.googleapis.com/golang/go1.3.3.linux-amd64.tar.gz \
  | tar -v -C /usr/local -xz

ENV GOPATH /opt
RUN go get github.com/mitchellh/gox
RUN gox -build-toolchain

# install requires go binaries
RUN go get github.com/tools/godep
RUN go get github.com/ddollar/forego

# load bundle to speed up Gem installs
ADD script/bundle_config /root/.bundle/config
RUN /usr/local/ruby/2.1.2/bin/gem install bundle
ADD ./nitrous/rails/Gemfile /opt/src/github.com/nitrous-io/dedicated/nitrous/rails/Gemfile
ADD ./nitrous/rails/Gemfile.lock /opt/src/github.com/nitrous-io/dedicated/nitrous/rails/Gemfile.lock
RUN cd ./nitrous/rails && /usr/local/ruby/2.1.2/bin/bundle install

RUN /etc/init.d/postgresql start &&\
  su postgres -c "psql -c 'CREATE ROLE root NOSUPERUSER CREATEDB NOCREATEROLE INHERIT LOGIN;'" &&\
  createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_test &&\
	su postgres -c "psql -d nitrous_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'"

# postgres settings for testing
RUN echo "fsync=off" >> /etc/postgresql/9.3/main/postgresql.conf && \
  echo "synchronous_commit=off" >> /etc/postgresql/9.3/main/postgresql.conf && \
  echo "full_page_writes=off" >> /etc/postgresql/9.3/main/postgresql.conf

# set GOPATH to include the path to dependencies
ENV GOPATH /opt/src/github.com/nitrous-io/dedicated/Godeps/_workspace:/opt
ADD . /opt/src/github.com/nitrous-io/dedicated
