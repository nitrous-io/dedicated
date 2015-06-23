# Nitrous Pro (Nitrous Dedicated) Central Repo

Contain components of the nitrous-dedicated platform.
- nitrous: the webapp
- hostbuilder: building the actual hosts
- freighter: part that lives in the host
- compass: the DNS
- tender: proxy to the hosts

### Set Up

Setting up a development environment for Nitrous is simple on Nitrous Pro.

1. Create a workspace. You need at least 2GB RAM. (Pro Recommended)

2. At the top-right, select "Account Settings" from your profile.

  ![screen shot 2015-03-30 at 2 10 39 pm](https://cloud.githubusercontent.com/assets/4488777/6891004/1aa286b2-d6e8-11e4-914b-4fb4b34a331e.png)

3. Enable Advanced Container Management under "Settings" and apply the changes.

  ![screen shot 2015-02-27 at 5 40 07 pm](https://cloud.githubusercontent.com/assets/30173/6424148/b45e5410-bea7-11e4-8d67-076ca8492b41.png)

3. Ensure you have access to Nitrous's repo on Quay.io.

4. Connect to the registry.

  ![screen shot 2015-03-30 at 11 38 20 am](https://cloud.githubusercontent.com/assets/4488777/6891061/ae25b792-d6e8-11e4-968b-d35b60ea5893.png)

5. Select Quay.io button for the registry URL and key in your credentials for your Quay.io account. Connect.

  ![screen shot 2015-03-30 at 11 37 48 am](https://cloud.githubusercontent.com/assets/4488777/6891049/947aa1cc-d6e8-11e4-9020-f81c7d680fbd.png)

6. You should see this:

  ![screen shot 2015-03-30 at 11 45 05 am](https://cloud.githubusercontent.com/assets/4488777/6891123/84b30e18-d6e9-11e4-95ab-163a5e75f367.png)

7. Next, create a container

  ![screen shot 2015-03-30 at 11 46 40 am](https://cloud.githubusercontent.com/assets/4488777/6891126/994b7810-d6e9-11e4-9b04-7677954d8b8b.png)

8. Specify the following image repository:

  ![screen shot 2015-03-30 at 11 48 06 am](https://cloud.githubusercontent.com/assets/4488777/6891134/b025d152-d6e9-11e4-8ba7-43a838109449.png)

9. In the container config screen, it is recommended that you create a fixed port binding for port 22, so that the SSH URL remains static. Click Add.

  ![screen shot 2015-02-27 at 5 43 48 pm](https://cloud.githubusercontent.com/assets/30173/6424176/56270724-bea8-11e4-9e15-4904ecdc8b64.png)

10. Privileged Mode must be enabled.

  ![screen shot 2015-02-27 at 5 43 54 pm](https://cloud.githubusercontent.com/assets/30173/6424178/5b0ec2b8-bea8-11e4-895b-312dabbaef3d.png)

11. Open the IDE, follow the instructions in the Readme.

12. Setup Rabbitmq. Replace the ports if necessary.

```
docker run -d -e RABBITMQ_NODENAME=my-rabbit \
  --name some-rabbit \
  -p 127.0.0.1:5672:5672 -p 8081:15672 rabbitmq:3-management
```

### Commands

```
- make: build binaries and assets
- make nitrous/freighter/hostbuilder: build independent binaries
- make test: test all apps
- make APP=(nitrous/freighter/hostbuilder) test: test individual app
- make image: make the image used for build and test
- make ci-test: test all apps in CI mode. (i.e. run tests inside a docker container)
- make APP=(nitrous/freighter/hostbuilder) ci-test: test individual app
- make ci-build: build all apps in CI mode.
- make APP=(nitrous/freighter/hostbuilder) ci-build: build individual app
- make release: release all apps
- make APP=(nitrous/freighter/hostbuilder) release: release individial app
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
