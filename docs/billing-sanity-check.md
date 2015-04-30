# Billing Sanity Check

The guide covers how to review the upcoming biller run and identify any potential issues (or unhappy customers)

### Grab a DB dump

You will need to grab the latest production database dump using the `heroku` toolkit.

First create a backup (this will take some time depending on the DB size)

```
heroku pg:backups capture --app pro-nitrous-io
```

The command will provide a identifier, in this case our identifier was `b007`.

Now obtain the downloadable URL for the created backup.

```
heroku pg:backups public-url b007  --app pro-nitrous-io
```

Now use your favourite CLI web tool (e.g. curl) to download the file

```
curl "URL" > ~/nitrous_prod_db7
```

### Restoring the nitrous_production DB

You will need to first create the database, if you wish to keep multiple production databases you can create a completely new one with some form of numbering.

```
createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_production7
```

Now you'll need to restore the dump into the database, where `nitrous_prod_db7` is the downloaded database and `nitrous_production7` is the target database.

```
pg_restore ~/nitrous_prod_db7 | psql -d nitrous_production7
```

You should now be able to query the database

```
> psql -d nitrous_production7
psql (9.3.6)
Type "help" for help.

nitrous_production7=# SELECT COUNT(*) FROM users;
 count
-------
  3114
(1 row)
```

### Running the biller

You will now need to run the biller to simulate a biller run

First make sure you're up to date with master and have built the latest binaries

```
➜  dedicated git:(master) ✗ git pull origin master
From github.com:nitrous-io/dedicated
 * branch            master     -> FETCH_HEAD
Already up-to-date.
➜  dedicated git:(master) ✗ make nitrous
```

To run the biller you'll need to obtain the Unix epoch time for the start and end date of this month in UTC (or whatever month range you'll need to simulate).

Tip: USAGE_START and USAGE_END can be derived from the `date` command. `date -d'2015-04-01' -u +%s` for beginning, `date -d'2015-04-30 23:59:59' -u +%s` for end. (This command works on Linux only, so use NiN)


You'll need to provide the following environment variables

* USAGE_START, USAGE_END: The start and end dates in Unix epoch seconds. If omitted it defaults to the previous month
* SENDGRID_USERNAME, SENDGRID_PASSWORD: Provide a bad value to ensure emails are not sent
* STRIPE_SECRET_KEY, STRIPE_PUBLIC_KEY: Provide bad values to ensure Stripe is not contacted successfully
* NITROUS_PG_URL: The DB url, set `dbname` to use the database you restored to.


```
USAGE_START=1427846400 USAGE_END=1430438399 SENDGRID_USERNAME=s SENDGRID_PASSWORD=s STRIPE_SECRET_KEY=disable STRIPE_PUBLIC_KEY=disable NITROUS_PG_URL="host=/var/run/postgresql user=nitrous dbname=nitrous_production7 sslmode=disable" build/biller &>/home/nitrous/code/biller_7.log
```

Now inspect the log for errors ("[ERROR]") (other than Stripe not being contactable)

After inspecting you'll want to see what the current invoices look like, be sure to substitute `period_start` for the start date you are generating invoices for.

```
SELECT invoices.id, username, round(total_charge/100.0, 2) AS charge, round(credit_amount/100.0, 2) AS credit, round(available_credit/100.0, 2) AS avail, state FROM invoices INNER JOIN users ON invoices.user_id = users.id WHERE period_start = '2015-04-01 00:00:00' ORDER BY total_charge DESC;
```

If you wish to only view the possible charged accounts

```
SELECT invoices.id, username, round(total_charge/100.0, 2) AS charge, round(credit_amount/100.0, 2) AS credit, round(available_credit/100.0, 2) AS avail, state FROM invoices INNER JOIN users ON invoices.user_id = users.id WHERE period_start = '2015-04-01 00:00:00' AND total_charge >= 100 AND state = 'unpaid' ORDER BY total_charge DESC;
```

You should then pick out a few invoices and cross reference them with the `invoice_items` table

```
SELECT * FROM invoice_items WHERE invoice_id = 478;
```


### Errors you should look at

##### [ERROR] biller: Pricing XXXX for user XXX host XXXX exceeds plan price ratio

This occurs when the total charge of a host exceeds the advertised price of a plan by the specified ratio in `biller.go`. The result of this is when plan pricing has been adjusted significantly that hosts that belong to that plan haven't been updated and violate the ratio.

The suggested fix for this is to migrate these users to the new pricing or move them to a sunsetted plan.

Failure to fix this will result in invoices **not** being generated for these users.

