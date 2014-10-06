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
- make TESTSUITE=(nitro/nitrous/freighter/hostbuilder) test: test individial app
- make image: make the image used for build and test
- make ci-test: test all apps in CI mode. (i.e. run tests inside a docker container)
- make ci-build: build all apps in CI mode.
- make createdb/dropdb: prepare the DB. Useful in development
```

# dependencies management

From commit 6b68b5fefdc064cc9c4a9e45474f6430924c6c3d

```
Vendoring dependencies makes the build more stable and remove a lot of
complexities when building without git credentials.  Successfully cloning a
project means that you can also build it.

When comparing with other tools, Godep excels in this approach because
it also removes the dependencies's metadata (.git, .hg). The simpler
alternative would be to just write a shell script to clone, but lets goes
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


