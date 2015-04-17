#!/bin/bash

set -o nounset
set -o errexit
set -o pipefail

function setup_etc_hosts() {
  cat << EOF >/etc/hosts
  172.17.0.18	13dce3084b21
  127.0.0.1	localhost n2odev.com n2odevapp.com
  ::1	localhost ip6-localhost ip6-loopback
  fe00::0	ip6-localnet
  ff00::0	ip6-mcastprefix
  ff02::1	ip6-allnodes
  ff02::2	ip6-allrouters
EOF
}

function cat_etc_resolv_conf() {
  echo "/etc/resolv.conf start"
  cat /etc/resolv.conf
  echo "/etc/resolv.conf end"
  exit
}

function setup() {
  setup_etc_hosts
  make tender
  /etc/init.d/postgresql start
  psql -d tender_test -f tender/testdata/fixtures.sql
}

setup
go run ./t/test_tender.go
