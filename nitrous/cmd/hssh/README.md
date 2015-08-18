### hssh

Perform administrative actions on a Nitrous.IO workspace.

#### Setup

You'll need to get SSH private keys corresponding to the EC2 keypairs used by
hostbuilder and put them in the path specified by the `SSH_KEYS_DIR` env var.
You can get them from the shared 1Password vault or from
[this data bag](https://github.com/nitrous-io/nitrous_cookbooks/blob/master/data_bags/ec2/keypairs.json).
The private key files should be named `<ec2 keypair name>.pem`, e.g.
`hostbuilder.ap-southeast-1.production.pem`.

#### Usage

```
export NITROUS_PG_URL=db_url
export AES_KEY=private_aes_key
export AWS_ACCESS_KEY=aws_access_key
export AWS_SECRET_KEY=aws_secret_key
export SSH_KEYS_DIR=/path/to/ec2-keypair-dir
export SSH_TIMEOUT=30s # optional

# open a ssh shell on host with slug awesome-host
hssh shell awesome-host

# get private key so you can run your own ssh or scp command
hssh info awesome-host

# upload a file to remote awesome-host.
hssh upload awesome-host /path/to/local-file:/remotepath ...

# run a command on remote awesome-host
hssh run awesome-host command
```
