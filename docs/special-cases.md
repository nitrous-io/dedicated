## Special cases

To document anything that deviates from expectations.  Whenever you see
something that doesn't 'look right', check here first.

### Features that don't work on dind

- host upgrade

Since dind hosts use a mounted `freighterd` and do not have
`/etc/apt/sources.list.d/nitrousio.list`, attempting to upgrade them will fail.

### Admin-only features

Features that appear on the default views but are invisible to non-admin users.

- 'bypass preprovisioner' checkbox in New Workspace

This allows us to bypass the preprovisioner and build a new host.  It is used
so we don't have to exhaust existing provisioned hosts to check if a creating a
new host works as intended: image for host has been updated, hostbuilder Create
code changed, etc.

- 'Docker ver' and 'Freighterd ver' on Workspace tab

Allows admins to tell at a glance which versions a host is on (particularly
useful when upgrading)
