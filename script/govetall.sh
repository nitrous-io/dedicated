#!/bin/bash

# this runs go vet on all source code, ignoring very common "using unkeyed struct"
# error.
# must be run from top-level dir (./scripts/govetall.sh)
export GOPATH=$(pwd)/Godeps/_workspace:$GOPATH

go tool vet . 2>&1 | grep --invert-match "unkeyed" | grep --invert-match "^Godeps"
