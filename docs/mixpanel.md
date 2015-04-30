# Mixpanel

## User profiles

### Adding a new property

Examples can be found at [PR
1398](https://github.com/nitrous-io/dedicated/pull/1398) and [PR
1401](https://github.com/nitrous-io/dedicated/pull/1401).

#### Backfill existing user profiles

Use the command in `nitrous/cmd/populatemxprofiles` to add a new property to
existing users.

By default `$ip = "0"` and `$ignore_time = true` will be set on each request.
The former means that the user's geolocation will not be updated (it will
change to the ip of the machine the command was run on otherwise).  The latter
means that the user's "last seen" property will not be updated.

Add a step to `steps.go`, change `main.go` to only run that step, and build.
Run on your local machine with the necessary environment variables taken from
`pro.nitrous.io:/etc/default/nitrous` (`NITROUS_PG_URL` and `MIXPANEL_TOKEN` at
time of writing).  Commit and push the changes (so we know what step has been
run).

#### Adding to frontend

Try to add profile properties in the frontend unless it is too complex.

Define a constant in `n/r/a/a/j/lib/mp-events.js.coffee`.  Run the appropriate
command from `n/r/a/a/j/lib/mp.js.coffee`.  These are:

- increment
- updateUser
- appendUser
- unionUser
- timestamp

Do take note of the data already in the profile -- for lists you generally want
to be using `appendUser` or `unionUser` so that existing data is not
overwritten.

#### Adding to backend

The backend mixpanel utils will automatically set $ip to "0".

Just call `env.Tracker.SetProfileProps` with the relevant arguments.  Examples
in `nitrous/controllers/github/github{,_test}.go`
