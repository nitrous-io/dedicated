Nitrous
=======

[![Build Status](https://ci-use1.nitrous.io/buildStatus/icon?job=nitrous)](https://ci-use1.nitrous.io/job/nitrous/)

## Setup

1. Install Postgres - `sudo apt-get install postgresql-9.3 postgresql-contrib-9.3 libpq-dev`
2. Create Postgres User - `sudo su postgres -c "psql -c \"CREATE ROLE "$(whoami)" NOSUPERUSER CREATEDB NOCREATEROLE INHERIT LOGIN;\""`
3. `make createdb`

## Running
1. `cp .env-example .env`
2. `make seed`
3. `make run` or `make go; goop exec forego start`

- - -
Copyright (c) 2014 Irrational Industries, Inc. d.b.a. Nitrous.IO. All Rights Reserved.
