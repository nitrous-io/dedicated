#!/bin/bash

set -e

dedi_path=/home/$NITROUS_USERNAME/code/go/src/github.com/nitrous-io/dedicated

if [[ ! -d $dedi_path ]]; then
  mkdir -p /home/$NITROUS_USERNAME/code/go/src/github.com/nitrous-io
  cd /home/$NITROUS_USERNAME/code/go/src/github.com/nitrous-io
  git clone git@github.com:nitrous-io/dedicated.git
fi

if [[ ! -e /home/$NITROUS_USERNAME/code/dedicated ]]; then
  cd /home/$NITROUS_USERNAME/code
  ln -s $dedi_path .
fi

if [[ ! -f $dedi_path/hostbuilder/config.json ]]; then
  cp $dedi_path/hostbuilder/{sample_,}config.json
fi

if [[ ! -f $dedi_path/compass/config.json ]]; then
  cp $dedi_path/compass/{sample_,}config.json
fi

if [[ ! -f $dedi_path/tender/.env ]]; then
  cp $dedi_path/tender/.env{-example,}
fi

if [[ ! -f $dedi_path/nitrous/.env ]]; then
  cp $dedi_path/nitrous/.env{-example,}
  sed -i 's|\${PWD}|/home/nitrous/code/dedicated/nitrous|' $dedi_path/nitrous/.env
fi

if ! sudo su postgres -c 'psql -d nitrous_development -c "SELECT 1"'; then
  cd $dedi_path
  make createdb
fi

if ! psql nitrous_development -c 'SELECT email FROM users' | grep admins@nitrous.io; then
  cd $dedi_path
  make nitrous
  cd nitrous
  forego run ../build/dbseed
  cd ..
fi

cd $dedi_path/nitrous/rails
bundle install

react_path=/home/$NITROUS_USERNAME/code/react

if [[ ! -d $react_path ]]; then
  git clone git@github.com:nitrous-io/nitrous-react.git $react_path
  cd $react_path
  # nvm is a definition, so need to run in subshell
  zsh -l -c 'nvm install v4.2.1 && nvm alias default v4.2.1'
  echo 'export PATH="node_modules/.bin:$PATH"' >> .zshrc
fi
