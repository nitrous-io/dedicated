#!/bin/bash

set -o nounset
set -o errexit
set -o pipefail

function setup() {
  make tender
  /etc/init.d/postgresql start
  psql -d tender_test -f tender/testdata/fixtures.sql
}

setup
go run ./t/test_tender.go
