#!/bin/bash

# Compiles all go projects in tree.
# For making sure code changes didn't break the compilation.
# -all cmd-line arg : also compile tests (much longer)
# Compiling projects:   ~20 secs
# Compiling with tests: ~3 mins

set -o nounset
set -o errexit
set -o pipefail

make image

time docker run --rm --privileged nitrousio/dedicated-dev:latest go run script/compile-all.go $@
