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

# different ways of trying to add our dev certificate so that it's recognized
# Not working
function setup_certificates2() {
  apt-get install -y ca-certificates
  mkdir -p /usr/share/ca-certificates/extra
  cp script/certs/n2odev.crt /usr/share/ca-certificates/extra
  cp script/certs/n2odevpro-ca.crt /usr/share/ca-certificates/extra
  update-ca-certificates
}

function setup_certificates3() {
  mkdir -p /usr/share/ca-certificates/extra
  cp script/certs/n2odev.crt /usr/share/ca-certificates/extra
  cp script/certs/n2odevpro-ca.crt /usr/share/ca-certificates/extra
  chmod 0644 /usr/share/ca-certificates/extra/n2odev.crt
  chmod 0644 /usr/share/ca-certificates/extra/n2odevpro-ca.crt
  ln -s /usr/share/ca-certificates/extra/n2odevpro-ca.crt /etc/ssl/certs/n2odevpro-ca.pem
  ln -s /usr/share/ca-certificates/extra/n2odev.crt /etc/ssl/certs/n2odev.pem

  ls -lah /usr/share/ca-certificates/mozilla/VeriSign_Class_3_Public_Primary_Certification_Authority_-_G5.crt
  ls -lah /usr/share/ca-certificates/extra/n2odev*
  ls -lah /etc/ssl/certs/n2odev*
  ls -lah /etc/ssl/certs/StartCom_Certification_Authority.pem
  dpkg-reconfigure -f noninteractive ca-certificates
}

function setup_certificates4() {
  cp script/certs/n2odev.crt /usr/local/share/ca-certificates
  cp script/certs/n2odevpro-ca.crt /usr/local/share/ca-certificates
  chmod 0644 /usr/local/share/ca-certificates/n2odev.crt
  chmod 0644 /usr/local/share/ca-certificates/n2odevpro-ca.crt
  #ls -lah /usr/local/share/ca-certificates/
  update-ca-certificates
  dpkg-reconfigure -f noninteractive ca-certificates
}

# On Ubuntu Go reads certificates from /etc/ssl/certs/ca-certificates.crt:
# https://sourcegraph.com/github.com/golang/go@ac452349e4f40c30d50f9922bf2c4592e748ce5e/.tree/src/crypto/x509/root_linux.go#startline=8&endline=8
# This rebuilds that file from certificates in /usr/share/ca-certificates and
# /usr/local/share/ca-certificates
function setup_certificates_1() {
  #ls -lah /etc/ssl/certs/ca-certificates.crt
  cp script/certs/n2odev.crt /usr/local/share/ca-certificates
  cp script/certs/n2odevpro-ca.crt /usr/local/share/ca-certificates
  chmod 0644 /usr/local/share/ca-certificates/n2odev.crt
  chmod 0644 /usr/local/share/ca-certificates/n2odevpro-ca.crt
  update-ca-certificates
  #ls -lah /etc/ssl/certs/ca-certificates.crt
}

function setup_certificates() {
  #ls -lah /etc/ssl/certs/ca-certificates.crt
  cp nitrous/dev/n2odev.crt /usr/local/share/ca-certificates
  cp nitrous/dev/n2odevpro-ca.crt /usr/local/share/ca-certificates
  chmod 0644 /usr/local/share/ca-certificates/n2odev.crt
  chmod 0644 /usr/local/share/ca-certificates/n2odevpro-ca.crt
  update-ca-certificates
  #ls -lah /etc/ssl/certs/ca-certificates.crt
}

function cat_etc_resolv_conf() {
  echo "/etc/resolv.conf start"
  cat /etc/resolv.conf
  echo "/etc/resolv.conf end"
  exit
}

function setup() {
  setup_etc_hosts
  setup_certificates
  #setup_certificates3
  make tender
  /etc/init.d/postgresql start
  psql -d tender_test -f tender/testdata/fixtures.sql
}

setup
go run ./t/test_tender.go
