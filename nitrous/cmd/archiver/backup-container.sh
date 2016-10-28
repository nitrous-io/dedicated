#!/bin/bash
set -e

: "${AWS_ACCESS_KEY_ID:?AWS_ACCESS_KEY_ID is required}"
: "${AWS_SECRET_ACCESS_KEY:?AWS_SECRET_ACCESS_KEY is required}"
: "${AWS_REGION:?AWS_REGION is required}"
: "${BUCKET:?BUCKET is required}"
: "${CONTAINER:?CONTAINER is required}"
: "${CONTAINER_HASH:?CONTAINER_HASH is required}"

if [[ ! -f ./gof3r ]]; then
  wget --quiet https://s3.amazonaws.com/nitrous-container-backups/gof3r -O ./gof3r
  chmod +x ./gof3r
fi

S3_KEY=$CONTAINER_HASH.tar.gz

docker cp $CONTAINER:/home/nitrous - | \
  gzip --stdout -8 | \
  ./gof3r put --bucket $BUCKET --key $S3_KEY --endpoint s3-accelerate.amazonaws.com --acl public-read

echo "SUCCESS. Container $CONTAINER backed up to s3://$BUCKET/$S3_KEY."

# TODO We might want to save container metadata like port mappings.
