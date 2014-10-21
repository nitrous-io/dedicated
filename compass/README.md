## Nitrous.IO Authoritative DNS Server

This accepts DNS requests to our boxhost domain and correctly routes them to
the correct server. Acts as the replacement for Route53 that doesnt suck, and
also doesnt requires us to use Route53 API

For now it will read PostgreSQL database to fetch data.

## Setup development environments
Because this depends on PostgreSQL to run the tests, we setup a docker image
with proper dependencies to run tests.

- `make setup` will install goop and the project dependencies
- `make shell` to enter the docker shell and run tests interactively. The
  source is bind-mounted into the docker container so changes will be reflected
  immediately
- To run test as intended for CI, use `make test`
- To build the binary for running locally, use `make build`. The new binary
  will be located in `build/compass`
- `make release` to release the binary to s3

- To run locally, change `sample_config.json` to `config.json`. As this should
  reuse nitrous's database, change the database url to such is probably a good
  idea.
