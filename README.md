Nitrous
=======

Nitrous On-Prem

## Setup

1. Install Postgres - `sudo apt-get install postgresql-9.3 postgresql-9.3-contrib libpq-dev`
2. Create Postgres User - `sudo su postgres -c "psql -c \"CREATE ROLE "$(whoami)" NOSUPERUSER CREATEDB NOCREATEROLE INHERIT LOGIN;\""`

- - -
Copyright (c) 2014 Irrational Industries, Inc. d.b.a. Nitrous.IO. All Rights Reserved.
