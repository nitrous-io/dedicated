## Updating user hosts

We use Ansible to mass run commands on all user hosts.

### Setup

#### Install Ansible

Instructions for Ubuntu
[here](https://docs.ansible.com/ansible/intro_installation.html#latest-releases-via-apt-ubuntu).

At time of writing, these were:

```
sudo apt-get install software-properties-common
sudo apt-add-repository ppa:ansible/ansible
sudo apt-get update
sudo apt-get install ansible
```

The version currently in use is `1.9.2`.

#### Configure

Copy the `ansible.cfg.sample` file to `~/.ansible.cfg`.  Depending on your
machine's specs, you may want to increase/decrease the number of forks.

#### Test your setup

Create a test inventory file using `inventory.sample` as a guide.

Run `ansible all -m ping -i /path/to/inventoryfile`.

### Generating inventory

Build the `hosts-inventory` command in `nitrous/cmd/hosts-inventory` and run it
against production.  This will write an `inventory` file and all the private
keys needed in a subdir of `/tmp`.  You can pass an argument to use a different
directory.  Previously written private keys are reused.

You will need to export these env vars:

* NITROUS_PG_URL
* AES_KEY
* SSH_KEYS_DIR

*NOTE*: this gives access to every single user host.  Only run this in a safe
environment.

### Running playbooks

The `primary.yml` file is the master playbook, which means all hosts are
expected to have whatever it runs.

It is highly advised that if you make any changes, you test on your hosts
first, using the a custom inventory file instead of the generated one.

Run the primary playbook with (assuming `~/code/dedicated` as pwd):

`ansible-playbook -i /path/to/inventoryfile userhosts/primary.yml`

