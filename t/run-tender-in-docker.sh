#!/bin/bash

set -o nounset
set -o errexit
set -o pipefail

make tender
/etc/init.d/postgresql start
psql -d tender_test -f tender/testdata/fixtures.sql
forego run -e test/.env-tender build/tender
