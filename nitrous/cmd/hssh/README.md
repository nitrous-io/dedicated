### ssh

Perform administrative actions on a remote Nitrous.IO host

```
export NITROUS_PG_URL=db_url
export AES_KEY=private_aes_key
export HOST_HOSTNAME=n2ostagingpro.com
export PROXY_HOSTNAME=n2ostagingproxy.com

# open a ssh shell on host with slug awesome-host
hssh shell awesome-host

# get private key so you can run your own ssh or scp command
hssh info awesome-host

# upload a file to remote awesome-host.
hssh upload awesome-host file:/remotepath ...

# run a command on remote awesome-host
hssh run awesome-host command
```
