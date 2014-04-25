## Hostbuilder

Provision a box host

## Config

```
{
  providers: [
    {
      name: "amazon",
      secret_key: ...,
      access_key: ...,
      ami_id: ami-number-here,
    }
  ]
}
```

### Endpoints

- `POST /host`: create a boxhost, with the following params

```
provider: amazon
config: json encoded value for the provider
```
