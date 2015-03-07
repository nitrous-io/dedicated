# Nitrous Pro (Nitrous Dedicated) Central Repo

Contain components of the nitrous-dedicated platform.
- nitrous: the webapp
- nitro: the CLI
- hostbuilder: building the actual hosts
- freighter: part that lives in the host
- compass: the DNS
- tender: proxy to the hosts

### Set Up

Setting up a development environment for Nitrous is simple on Nitrous Pro.

1. Create a workspace. You need at least 2GB RAM. (Nitro Recommended)
2. Enable Advanced Container Management

  ![screen shot 2015-02-27 at 5 40 07 pm](https://cloud.githubusercontent.com/assets/30173/6424148/b45e5410-bea7-11e4-8d67-076ca8492b41.png)
3. Log into Quay.io

  ![screen shot 2015-02-27 at 5 39 58 pm](https://cloud.githubusercontent.com/assets/30173/6424150/c820f9bc-bea7-11e4-8822-a0d6c1c70b54.png)
4. Go to new container screen, specify `quay.io/nitrous/nitrous-on-nitrous` image and click next.

  ![screen shot 2015-02-27 at 5 41 13 pm](https://cloud.githubusercontent.com/assets/30173/6424153/e121f632-bea7-11e4-996f-99bcdc423714.png)

5. In the container config screen, it is recommended that you create a fixed port binding for port 22, so that the SSH URL remains static. Click Add.

  ![screen shot 2015-02-27 at 5 43 48 pm](https://cloud.githubusercontent.com/assets/30173/6424176/56270724-bea8-11e4-9e15-4904ecdc8b64.png)

  Privileged Mode must be enabled.

  ![screen shot 2015-02-27 at 5 43 54 pm](https://cloud.githubusercontent.com/assets/30173/6424178/5b0ec2b8-bea8-11e4-895b-312dabbaef3d.png)

6. Open the IDE, follow the instructions in the Readme.

### Commands

```
- make: build binaries and assets
- make nitro/nitrous/freighter/hostbuilder: build independent binaries
- make test: test all apps
- make APP=(nitro/nitrous/freighter/hostbuilder) test: test individual app
- make image: make the image used for build and test
- make ci-test: test all apps in CI mode. (i.e. run tests inside a docker container)
- make APP=(nitro/nitrous/freighter/hostbuilder) ci-test: test individual app
- make ci-build: build all apps in CI mode.
- make APP=(nitro/nitrous/freighter/hostbuilder) ci-build: build individual app
- make release: release all apps
- make APP=(nitro/nitrous/freighter/hostbuilder) release: release individial app
- make createdb/dropdb: prepare the DB. Useful in development
```

### Dependency management

We use [Godep](https://github.com/tools/godep) and vendor dependencies (i.e.
dependencies are checked in).

#### Using Godep

* Install Godep: `go get github.com/tools/godep`
* Copy the vendored dependencies in `Godeps/` to your `GOPATH`: `godep restore`
* To add a new dependency:
  1. `go get foo/bar`
  * Edit your code to import `foo/bar`.
  * `godep save ./...`
  * Verify the changes and commit the changes in `Godeps/` in a separate commit.

NOTE: if you change your GOPATH to something like $GOPATH:$(godep path),
remmeber to reset it to global GOPATH before get/update/save the dependencies

#### Why Godep?

We use Godep for now, until the Go community recommends an official approach.

See https://groups.google.com/d/msg/golang-dev/nMWoEAG55v8/iJGgur7W_SEJ.

- - -
Copyright (c) 2012-2015 Nitrous, Inc. All Rights Reserved.
