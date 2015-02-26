#!/bin/bash

set -e

dedi_path=/home/$NITROUS_USERNAME/code/dedicated

if [[ ! -d $dedi_path ]]; then
  cd /home/$NITROUS_USERNAME/code
  git clone git@github.com:nitrous-io/dedicated.git
fi

mkdir -p /home/$NITROUS_USERNAME/code/go/src/github.com/nitrous-io
if [[ ! -e /home/$NITROUS_USERNAME/code/go/src/github.com/nitrous-io/dedicated ]]; then
  cd /home/$NITROUS_USERNAME/code/go/src/github.com/nitrous-io
  ln -s $dedi_path .
fi

if [[ ! -f $dedi_path/hostbuilder/config.json ]]; then
  cp $dedi_path/hostbuilder/{sample_,}config.json
  sed -i 's|vagrant|nitrous|' $dedi_path/hostbuilder/config.json
  sed -i 's|workspace|code|' $dedi_path/hostbuilder/config.json
fi

if [[ ! -f $dedi_path/compass/config.json ]]; then
  cp $dedi_path/compass/{sample_,}config.json
  sed -i 's|vagrant|nitrous|' $dedi_path/compass/config.json
fi

if [[ ! -f $dedi_path/tender/.env ]]; then
  cp $dedi_path/tender/.env{-example,}
  sed -i 's|vagrant|nitrous|' $dedi_path/tender/.env
fi

if [[ ! -f $dedi_path/nitrous/.env ]]; then
  cp $dedi_path/nitrous/.env{-example,}
  sed -i 's|vagrant|nitrous|' $dedi_path/nitrous/.env
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
