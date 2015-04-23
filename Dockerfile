From nitrousio/ubuntu-dind:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y --no-install-recommends \
  apparmor \
  build-essential \
  curl \
  git-core \
  mercurial \
  postgresql-9.3 \
  postgresql-contrib-9.3 \
  libpq-dev \
  nodejs \
  wget

# Get the "busybox" image source so we can build locally instead of pulling
RUN git clone -b buildroot-2014.02 https://github.com/jpetazzo/docker-busybox.git /docker-busybox

# Get "ubuntu-bare" compressed image so that we can load it instead of pulling
RUN wget https://github.com/nitrous-io/docker-ubuntu-bare/releases/download/ubuntu-bare-v1/ubuntu-bare.tar.gz -O /root/ubuntu-bare.tar.gz

# get latest docker binary
RUN wget https://get.docker.com/builds/Linux/x86_64/docker-1.4.1 -O /usr/bin/docker && chmod +x /usr/bin/docker

RUN locale-gen en_US.utf8

# set paths
ENV HOME /root
ENV GOBIN /opt/bin
ENV PATH /opt/bundle/binstubs:/usr/local/go/bin:/opt/bin:$PATH
WORKDIR /opt/src/github.com/nitrous-io/dedicated

# install ruby from source
ENV RUBY_MAJOR 2.1
ENV RUBY_VERSION 2.1.4
RUN apt-get update \
  && apt-get install -y bison ruby autoconf \
  && rm -rf /var/lib/apt/lists/* \
  && mkdir -p /usr/src/ruby \
  && curl -SL "http://cache.ruby-lang.org/pub/ruby/$RUBY_MAJOR/ruby-$RUBY_VERSION.tar.bz2" \
    | tar -xjC /usr/src/ruby --strip-components=1 \
  && cd /usr/src/ruby \
  && autoconf \
  && ./configure --disable-install-doc \
  && make -j"$(nproc)" \
  && apt-get purge -y --auto-remove bison ruby autoconf \
  && make install \
  && rm -r /usr/src/ruby

# install go
RUN curl -s https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz \
  | tar -C /usr/local -xz

ENV GOPATH /opt

# install requires go binaries
RUN go get github.com/tools/godep
RUN go get github.com/ddollar/forego

# load bundle to speed up Gem installs
ADD script/bundle_config /root/.bundle/config
RUN gem install --no-ri --no-rdoc bundler
ADD ./nitrous/rails/Gemfile /opt/src/github.com/nitrous-io/dedicated/nitrous/rails/Gemfile
ADD ./nitrous/rails/Gemfile.lock /opt/src/github.com/nitrous-io/dedicated/nitrous/rails/Gemfile.lock
RUN cd ./nitrous/rails && bundle install

RUN /etc/init.d/postgresql start &&\
  su postgres -c "psql -c 'CREATE ROLE root SUPERUSER CREATEDB CREATEROLE INHERIT LOGIN;'" &&\
  createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_test &&\
	su postgres -c "psql -d nitrous_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" &&\
  createdb -E UTF8 -T template0 -l en_US.utf8 tender_test &&\
	su postgres -c "psql -d tender_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" &&\
  createdb -E UTF8 -T template0 -l en_US.utf8 compass_test &&\
	su postgres -c "psql -d compass_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'"

# postgres settings for testing
RUN echo "fsync=off" >> /etc/postgresql/9.3/main/postgresql.conf && \
  echo "synchronous_commit=off" >> /etc/postgresql/9.3/main/postgresql.conf && \
  echo "full_page_writes=off" >> /etc/postgresql/9.3/main/postgresql.conf

# set GOPATH to include the path to dependencies
ENV GOPATH /opt/src/github.com/nitrous-io/dedicated/Godeps/_workspace:/opt
ADD . /opt/src/github.com/nitrous-io/dedicated
