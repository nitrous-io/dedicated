# Hotswapping Explained

## What is it?

"Hotswapping" is the concept of attaching and detaching an EBS volume to a
running EC2 instance. This allows us to stop and start Nitrous workspaces
without actually stopping or starting EC2 instances.

## Moving parts

These are the moving parts required to make hotswapping work:

1. An AMI with a root volume and a user volume. See the [AMI details](#ami-details)
section for more info.
* Instance pools for each EC2 region and instance type that we use. See the
[Instance pools](#instance-pools) section for more info.
* A Redis instance used as a distributed lock for instance pools (so that
multiple processes can concurrently acquire/release instances to/from pools).
* Hostbuilder.

## AMI details

We build AMIs for hostbuilder to use to create new instances, using
[dedicated_boxhost_cookbook](https://github.com/nitrous-io/dedicated_boxhost_cookbook)
and [dedicated_builder](https://github.com/nitrous-io/dedicated_builder).

### AMI setup

1. Root volume `/dev/sda1` in block device mapping.
  * The OS and other non-user-specific files are stored here.
* User volume `/dev/sdu` in block device mapping.
  * User-specific files are stored here, specifically:
    * `/var/lib/docker` - Docker data directory
    * `/var/lib/freighter` - Freighter data directory (container secrets and volumes)
    * `/etc/freighterd` - Freighter config, certs, host secret
    * `/etc/apt/sources.list.d/nitrousio.list` - Apt repo for Docker and Freighter packages
* The user volume (represented by `/dev/xvdu` in the OS) is mounted at
`/mnt/userdata`.
* `/dev/xvdu -> /mnt/userdata` is added to `/etc/fstab` so that the volume is
mounted on boot.
* Upstart job at `/etc/init/resize-user-volume.conf` that resizes user volume
to its full size when its filesystem is mounted.
* Symlinks are created from the root volume to the user volume:
  * `/var/lib/docker` symlinks to `/mnt/userdata/var/lib/docker`
  * `/var/lib/freighter` symlinks to `/mnt/userdata/var/lib/freighter`
  * `/etc/freighterd` symlinks to `/mnt/userdata/etc/freighterd`
  * `/etc/apt/sources.list.d/nitrousio.list` symlinks to `/mnt/userdata/nitrousio.list`

With this setup, all hostbuilder needs to do when attaching a user volume is to
mount it and start Docker and Freighter.

## Instance pools

An instance pool is a group of floating EC2 instances without an attached user
volume. There is an instance pool for each EC2 region and instance type that we
use, and they are configured in `hostbuilder/config.json`.

Instances in the instance pool:

1. are named `userhost-pool`,
2. have a tag `dedicated:pool` with value `true`,
3. have a tag `dedicated` with value the appropriate environment
(e.g. `production`/`staging`/`development`).
4. are running.

At this time, the instance pools are not actively managed, i.e.:

* An instance is released to a pool when its workspace is stopped.
* An instance is removed from the pool when we create or start a workspace of
the pool's instance type.

There is no active pool watcher to keep it filled with instances.

## Hostbuilder

Hostbuilder manages the hotswapping, and is also backwards-compatible with
workspaces that do not support hotswapping (i.e. previously created workspaces
that only have a single volume).

### Creating a workspace

There are 2 possible cases:

##### 1. AMI supports user volume and acquired instance from instance pool

1. Create new user volume from AMI's snapshot of a blank slate user volume.
2. Attach volume to instance from pool.
3. SSH into instance
4. Mount the user volume.
5. Start Docker and Freighter.
6. Proceed as usual (e.g. associate EIP, write config files, etc.)

##### 2. AMI supports user volume but failed to acquire from instance pool

1. Create new instance using AMI.
2. For hotswap AMIs with 2 volumes, we set the size of the user volume to the
requested size (from nitrousd).
3. For non-hotswap AMIs with a single root volume, we set the size of the root
volume to the requested size.
4. Proceed as usual (e.g. associate EIP, write config files, etc.)

### Stopping a workspace

There are 2 possible cases:

##### 1. Workspace supports hotswapping

1. SSH into instance.
2. Unmount user volume.
3. Stop Docker and Freighter.
4. Detach user volume from instance.
5. Release instance to the pool. If pool is full, terminate the instance.
6. If a failure occurs during any step, we restore the workspace to its original
condition (e.g. re-attach volume, start Docker, start Freighter). This allows
the workspace to be safely stopped again.

##### 2. Workspace does not support hotswapping

1. Stop instance using EC2 API.

### Starting a workspace

There are 3 possible cases:

##### 1. Workspace supports hotswapping and acquired instance from instance pool

1. Attach user volume to instance from pool.
2. SSH into instance.
3. Mount the user volume.
4. Start Docker and Freighter.

##### 2. Workspace supports hotswapping but failed to acquire from instance pool

1. Create a new instance.
2. SSH into instance.
3. Unmount user volume.
4. Stop Docker and Freighter.
5. Detach and delete this blank slate volume from instance.
6. Attach user volume to instance.
7. SSH into instance.
8. Mount user volume.
9. Start Docker and Freighter.

##### 3. Workspace does not support hotswapping

1. Start instance using EC2 API.

### Terminating an instance

There are 3 possible cases:

##### 1. Workspace supports hotswapping and does not have an assigned instance

1. Delete user volume.

##### 2. Workspace supports hotswapping and has an assigned instance

1. Terminate instance.
2. Delete user volume.

##### 3. Workspace does not support hotswapping

1. Terminate instance.

### Respeccing an instance

There are 4 possible cases:

##### 1. Workspace supports hotswapping and is running

1. SSH into instance.
2. Unmount user volume.
3. Stop Docker and Freighter.
4. Take snapshot of user volume.
5. Create new (larger) volume from snapshot.
6. Detach old volume, attach new larger volume.
7. SSH into instance.
8. Mount user volume.
9. Start Docker and Freighter.
10. Delete old volume.

##### 2. Workspace supports hotswapping and is stopped

1. Take snapshot of user volume.
2. Create new (larger) volume from snapshot.
3. Delete old volume.
4. Return new volume ID to nitrousd.

##### 3. Workspace does not support hotswapping and is running

1. Stop instance.
2. Take snapshot of root volume.
3. Create new (larger) volume from snapshot.
4. Detach old volume, attach new larger volume.
5. Delete old volume.
6. Start instance.

##### 4. Workspace does not support hotswapping and is stopped

1. Take snapshot of root volume.
2. Create new (larger) volume from snapshot.
3. Detach old volume, attach new larger volume.
4. Delete old volume.
5. Start instance.
