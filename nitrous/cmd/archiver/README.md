# Nitrous Workstation Archiver

Use this to archive Nitrous.io workstations to S3. This will backup a gzipped tarball of the home folder of the `nitrous` user, for each of the containers on
the workstation.

This script must be run from a location where it can access Hostbuilder (so that
we can start any stopped workstations), e.g. from the same server where a
production Hostbuilder is running.

For example:

1. Build binary (statically linked):
   `CGO_ENABLED=0 GOOS=linux godep go build -a --ldflags '-extldflags "-static"' -o archiver main.go`
* Upload binary and required shell script:
  `scp -P65100 archiver backup-container.sh ubuntu@pro.nitrous.io:/home/ubuntu/`
* Create a `.env` file (make a copy of the `.env-example`) and fill in the
  correct values.
* Run the binary (you may want to pipe stdout to log file):
  `env $(cat .env | xargs) ./archiver`
* Each workstation will be logged in a separate file in a `logs/` directory,
  e.g. host ID 123 will be logged to `logs/host-123.log`.

The filenames of the tarballs will include a long hash composed by computing the
SHA256 digest of `<container slug>-<user encrypted password>`. Since the user's
password is used, the filenames are only valid if the user doesn't change their
password (i.e. please run this after we have shutdown access to Nitrous).
