#!/bin/bash
set -o nounset
set -o errexit
set -o pipefail

godep go run pkg/finder/cmd/find/main.go "$@"
