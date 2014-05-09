## Hostbuilder

Provision a box host

## Development

This vendor dependencies into `vendor` folder so we can easily change the
dependencies without changing import path. To fetch dependencies, use `make
setup` command.

Then you probably will need to change how GOPATH is defined in the shell
profile. I use autoenv (https://github.com/kennethreitz/autoenv)

## Config

```
{
  providers: {
    "amazon": {
      secret_key: ...,
      access_key: ...,
    }
  , "digitalocean": {
      client_id: ...,
      api_key: ...,
      aws_secret_key: ...,
      aws_access_key: ...,
      zone_id: ...,
      base_domain: ...
    }
  }
}
```

For digitalocean, we also requires aws access key and secret key because we use
Route53 as our DNS server, and we want to create a A record route to our
digitalocean instance i.e daniel-box-1234.nitrousbox.com

### Endpoints

- `POST /host`: create a boxhost, with the following params

```
provider: amazon
webhook: "https://hostbuild.er/webhook",
config: json encoded value for the provider
```

For amazon, the config is:

```ruby
{
  source_ami: "ami-167b1426",
  region: "us-west-2",
  security_group: {
    id: "sg-9b4c04ab",
    name: "box_host"
  },
  availability_zone: "us-west-2a",
  instance_type: "t1.micro",
  public_keys: [
    File.read("#{ENV['HOME']}/.ssh/id_rsa.pub")
  ]
}
```

For digitalocean, the config is:

```ruby
{
  name: "user-box-1",
  image_id: "3240036", # Ubuntu 14.04 x64
  region_id: "6", # Singapore 1
  size_id: "66", # 512MB
  public_keys: [
    File.read("#{ENV['HOME']}/.ssh/id_rsa.pub")
  ]
}
```
