# dedicated

Contain components of the nitrous-dedicated platform.
- nitrous: the webapp
- nitro: the CLI
- hostbuilder: building the actual hosts
- freighter: part that lives in the host

This doesn't contain compass/tender because they can have their own release
schedule.

# commands

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
- make createdb/dropdb: prepare the DB. Useful in development
```

# Dependency management

We use [Godep](https://github.com/tools/godep) and vendor dependencies (i.e.
dependencies are checked in).

## Using Godep

* Install Godep: `go get github.com/tools/godep`
* Copy the vendored dependencies in `Godeps/` to your `GOPATH`: `godep restore`
* To add a new dependency:
  1. `go get foo/bar`
  * Edit your code to import `foo/bar`.
  * `godep save ./...`
  * Verify the changes and commit the changes in `Godeps/` in a separate commit.

## Why Godep?

```
Vendoring dependencies makes the build more stable and removes a lot of
complexities when building without git credentials. Successfully cloning a
project means that you can also build it.

Compared to other tools, Godep excels in this approach because
it also removes the dependencies's metadata (.git, .hg). The simpler/more
manual alternative would be to just write a shell script to clone, but lets go
with this for now.

Resolving dependencies now lies completely in development phase. Once the
dependencies is set up correctly and committed, everyone will have the same
version of the world when building the binaries all the time. This is
particularly different from `Goopfile + Goopfile.lock` where update can pull in
unexpected (new version) of dependencies ( of dependencies ).

However, Godep also doesnt seem to resolve private fork of a public
dependencies ( AFAIK ?). A common approach when doing this in Go projects is to
commit the change directly into the GOPATH (i.e. Godeps/_workspace) since it's
checked in, and update the dependencies when the change is merged upstream.
```
