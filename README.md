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

12. Setup Stripe

All hosts created on Nitrous must have a Stripe subscription, for this work locally you will need to setup a Stripe account for testing.

* Set the following in your `nitrous/.env`

```
STRIPE_SECRET_KEY=sk_test_sixQCrN2afLit2BJVn1Uemiz
STRIPE_PUBLIC_KEY=pk_test_v7b75uFe7AAmLVoWmrInnWYc
```

* For each plan in the `plans` table you will need to create 2 plans in Stripe: one with an `ID` that matches the `plans.slug` and one where the ID matches `plans.slug`-exempt. Example: if the slug of plan matches `basic` in the table then you would create a Stripe plan with an ID of `basic` and another plan with an ID of `basic-exempt` that has a price of 0.
* Now run `UPDATE plans SET type = 'prepaid', stripe_plan_id = slug;`
* You can now use the test cards provided by Stripe when you are required to add a card: https://stripe.com/docs/testing#cards
* Otherwise you can set your billing profile in the database to the `state` of `exempt`.

##### Stripe Hooks

For the complete development flow it is recommended you setup Stripe webhooks. To achieve this you will need to do the following:

* Create this file (you will need root) in `/etc/nginx/sites-enabled/stripe-hook`

```
server {
  listen 6900;
  location / {
    deny all;
  }
  location /api/v0/stripe_hook {
                proxy_pass http://127.0.0.1:3333;
                proxy_http_version 1.1;
                proxy_set_header Upgrade $http_upgrade;
                proxy_set_header Connection $connection_upgrade;
        }
}
```

* Restart Nginx: `/etc/init.d/nginx restart`
* Expose the port `6900` on your container
* Go to Stripe > Login as your Nitrous and switch your account (top right) to "Nitrous Pro Dev"
* If you don't see it, ping someone on Slack for access
* Under Account Settings > Webhook specify the address of the container, e.g: `http://my-container-slug.nitrousapp.com:6900/api/v0/stripe_hook` as a Test web hook

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
- script/create-migration: creates a migration file to populating SQL
```

### Migrations

Much like Rails, migrations are now stored in individual files located in `nitrous/migrations`. The follow the format `1440095634_create_users_table.sql`, where `1440095634` is the current Unix Epoch Timestamp and `create_users_table` is a textual description of the what the migration does.

You can use the script `script/create-migration` to easily create your migrations.

Prior to this migrations were stored in `nitrous/db/migrations.go`.

### Dependency management

We use [Godep](https://github.com/tools/godep) and vendor dependencies (i.e.
dependencies are checked in).

### Add google DNS

On Mac, `Open System Preferences > Network`

Open the DNS tab, and add the following entries:

8.8.4.4
8.8.8.8

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

### Setup for GitHub OAuth with webhooks

We can test with HTTPS through preview URL to test GitHub OAuth with webhooks

* Change nginx config
  - /etc/nginx/sites-enabled/default
  ```conf
    listen 7000 default_server; # listen 7000 ssl default_server;

    #  ssl_certificate /etc/nginx/certs/n2odev.crt;
    #  ssl_certificate_key /etc/nginx/certs/n2odev.key;
    #  server_name pro.n2odev.com;
  ```
  - This is to disable `pro.n2odev.com` CA and use `*.nitrousapp.com` CA

* Change asset host in rails app
  - Update `<Dedicated Folder>/nitrous/rails/config/environments/development.rb`
  - i.e. when the container name is `nitrous-on-nitrous-9277`
  ```
    config.action_controller.asset_host = 'nitrous-on-nitrous-9277.nitrousapp.com' # '//pro%d.n2odev.com:7000'
  ```
  - This is to change assets urls for development.
* Add your `<Container Name>.nitrousapp.com` to GitHub Application.
  - `https://github.com/settings/developers`
  - Update `<Dedicated Folder>/nitrous/env` to have the credentials

* Reconfigure preview port
  - Option 1: your container to have 32700(any port you want) -> 7000 (TCP, Public) and update database
    - i.e. `update containers set port_mappings = '{"22":"32775","preview":"32700"}' where slug = 'nitrous-on-nitrous-9277';`
    - Wait for 1 minute to update atlas(proxy)
  - Option 2: Set `NITROUS_PREVIEW_PORT` to 7000

NOTE: Since your dev nitrous server become public, other people can access to your dev site.
Please shutdown the server or switch to original setting after you finish the testing.

- - -
Copyright (c) 2012-2015 Nitrous, Inc. All Rights Reserved.
