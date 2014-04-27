## Hostbuilder

Provision a box host

## Config

```
{
  providers: {
    "amazon": {
      secret_key: ...,
      access_key: ...,
    }
  }
}
```

### Endpoints

- `POST /host`: create a boxhost, with the following params

```
provider: amazon
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
