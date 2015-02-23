FROM quay.io/nitrous/docker:latest
MAINTAINER Nitrous, Inc. <eng@nitrous.io>

RUN \
  apt-get update && \
  apt-get install -yq --only-upgrade libcgmanager0 && \
  apt-get install -yq --no-install-recommends build-essential curl git-core mercurial dnsmasq && \
  apt-get install -yq postgresql-9.3 postgresql-contrib-9.3 libpq-dev nginx

EXPOSE 80
EXPOSE 443

RUN service postgresql start && \
  su postgres -c 'createdb nitrous_development && \
  createuser --superuser $NITROUS_USERNAME' && \
  service postgresql stop

ENV RUBY_VERSION 2.1.4
ENV RUBY_BIN /usr/local/ruby/$RUBY_VERSION/bin

RUN /bin/bash -l -c 'mkdir -p /usr/local/ruby/$RUBY_VERSION && \
  curl -s https://s3.amazonaws.com/pkgr-buildpack-ruby/current/ubuntu-14.04/ruby-$RUBY_VERSION.tgz \
    | tar -C /usr/local/ruby/$RUBY_VERSION -xz'

RUN \
  echo 'export PATH=$RUBY_BIN:$PATH' >> /etc/profile && \
  echo 'export PATH=.bundle/binstubs:$PATH' >> /etc/profile

USER $NITROUS_USERNAME

RUN /bin/bash -l -c 'mkdir -p ~/.bundle && \
  echo "BUNDLE_PATH: .bundle" > ~/.bundle/config && \
  echo "BUNDLE_BIN: .bundle/binstubs" >> ~/.bundle/config'

RUN /bin/bash -l -c 'if [[ ! -d $GOPATH/src/code.google.com/p/go.tools/cmd/goimports ]]; \
then go get code.google.com/p/go.tools/cmd/goimports; \
fi'

RUN /bin/bash -l -c 'if [[ ! -d $GOPATH/src/github.com/tools/godep ]]; \
then go get github.com/tools/godep; \
fi'

RUN /bin/bash -l -c 'if [[ ! -d $GOPATH/src/github.com/ddollar/forego ]]; \
then go get github.com/ddollar/forego; \
fi'

USER root

RUN /bin/bash -l -c '$RUBY_BIN/gem install bundler --no-ri --no-rdoc'

ADD nginx.conf /etc/nginx/nginx.conf

ADD setup.sh /home/$NITROUS_USERNAME/setup.sh
RUN chown $NITROUS_USERNAME:$NITROUS_USERNAME /home/$NITROUS_USERNAME/setup.sh
RUN chmod +x /home/$NITROUS_USERNAME/setup.sh

USER $NITROUS_USERNAME

ENV WORKSPACEDIR /home/$NITROUS_USERNAME/code/workspace
RUN mkdir $WORKSPACEDIR

USER root
