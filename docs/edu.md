## Edu(cation)

The `edu` flag is a property of both `orgs` and `plans`. When set on a `Org` it allows access to only `plans` with the `edu` flag set to true.

### How a Org becomes an `edu`

Because orgs are created in the same flow as they subscribe to a plan (the new multi host subscription model) we don't have a chance for Nitrous employees to set the `edu` flag on `Org` for them to gain access to the plans. Instead we need to preemptively set the `edu` flag during the creation of an `Org`, this is performed in `controllers/orgs/orgs.go#Create`.

The table `edu_emails_whitelist` provides this functionality by containing a list of emails that when used as the Org's `email` the `Org` is automatically flagged as `edu`. All a Nitrous engineer needs to do is insert into the table:

```sql
INSERT INTO edu_emails_whitelist(email) VALUES ('john@citizen.org');
```

The person creating the org must then use this email when entering in the `email` field (currently called "Billing Email" in the React application).

The `POST /orgs` (`controllers/orgs/orgs.go#Create`) also accepts an `intent` param. If the value is `edu` and the email is not found in `edu_emails_whitelist` then the route returns an error that can be handled by the web application. If this `intent` is not provided then the org creation will continue (otherwise that would break org creation entirely.)

NOTE: Currently rows are not removed from `edu_emails_whitelist` when claimed.

### Subscriptions

The `canSubscribe` func in `subscriptions/subscriptions.go` provides the check for ensuring that only `edu` orgs can subscribe to `edu` plans. Non edu orgs cannot subscribe to edu plans and edu orgs cannot subscribe to non edu plans.
