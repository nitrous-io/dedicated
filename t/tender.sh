#!/bin/bash

set -o nounset
set -o errexit
set -o pipefail

# VERSION=$(git rev-parse --short HEAD)

make image
docker run --privileged=true --rm nitrousio/dedicated-dev:latest \
  /bin/bash -c ./t/tender-in-docker.sh
