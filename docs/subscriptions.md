# Subscriptions

Subscription billing (or prepaid billing) is the current way all users pay for Nitrous. Subscription billing is backed by Stripe's subscription type billing and for every single subscription we create in Nitrous we have a corresponding subscription in Stripe (regardless of free or exempt billing states).

Subscriptions require the following Stripe functionality to work:

* webhooks: we use Stripe webhooks to notify when a subscription is updated, deleted or when a Stripe invoice is created & paid
* plans: for every plan in Nitrous we create the subsequent plan in Stripe


## Life cycle

### User adds card

When a user or org adds a card for the first time we activate their billing profile, which involves creating the Stripe customer record with the token provided by Stripe.js.

### User updates card

When a user or org wishes to update a card, we just update the existing Stripe record.

### Creating a workspace

We take a payment on delivery approach for billing users for workspaces. We create the workspace first and once we receive a successfully created message (in the queue handlers) we create the subscription for user or org in Stripe. Currently if the subscription creation fails (card failure or error) we terminate the host immediately.

`subscriptions.Subscribe` is responsible for creating this subscription. If a billing profile is exempt then it will use a Stripe plan ID suffixed with `-exempt`. If a user has any positive credits located in the `credits` table then these will be credited to the account as invoice items. Similarly if the current user has selected a plan that is not free a coupon will be used for the user if they have a coupon assigned against them.

### Cancelling a subscription

Unless you are a free user or an admin when a user or org cancels a subscription we call on Stripe to cancel that subscription at the end of the billing cycle. When the actual cancellation comes around Stripe then issues a subscription deleted event to our webhook where we then stop the host. 48 hours late the command `hostautoterminate` will pick up these cancelled hosts and terminate them.

`subscriptions.Cancel` is responsible for calling the cancel. It just makes a call to the Stripe endpoint for cancelling a subscription, webhooks located in `subscriptions/handlers.go` then handle the appropriate deletion event (whenever the actual Subscription delete occurs).

### Upgrading a subscription

Currently a subscription can only upgrade upwards between supported plans. We look at the existing plan and then check if they can upgrade to the next plan using `plan.CanUpgradeToPlan`, we then submit the update to Stripe which will perform the necessary pro-rata calculation. Much like creating a subscription we need to use the `-exempt` prefix and apply any credits. Because Stripe subscriptions don't immediately charge (unless you are charging for the first time) we need to request that the subscription be immediately paid to verify the legitimacy of the request. If the payment fails then we return an error.

If the subscription succeeds we then update the hosts for the new plan details and create respec records, the frontend is then responsible for performing the respecs - we do not upgrade and respec in the same call. The Ember frontend makes these calls as necessary: it upgrades and then calls respecs as needed.

`subscriptions.Upgrade` is responsible for performing the upgrade.

## Webhooks

Webhooks are extensively used so administrative actions can be taken on the Stripe dashboard and have them correctly affect the backend. The following events are currently handled by `subscriptions/handlers.go`:

* `invoice.payment_succeeded`: handled by `invoice.PaymentSucceeded`, marks when invoices succeed so a billing profile state can be corrected
* `invoice.payment_failed`: handled by `subscriptions.HandleSubscriptionInvoiceFailed` (in `subscriptions/handlers`) so it can update any delinquency state and send emails as needed
* `customer.subscription.updated`: handled by `subscriptions.HandleSubscriptionUpdated` (in `subscriptions/handlers`) so it can update a subscription when the billing period changes or is marked for deletion.
* `customer.subscription.deleted`: handled by `subscriptions.HandleSubscriptionDeleted` (in `subscriptions/handlers`) when Stripe notifies the subscription has ended (the billing period has ended and it was marked for deletion)
* invoice.created: handled by `subscriptions.HandleSubscriptionInvoiceCreated` (in `subscriptions/handlers`) when Stripe has notified a invoice has just been created (say a subscription has been renewed). Because Stripe notifies an hour before this is our chance to apply any credits as invoice items if the user has them available.

## Plans

For every plan in Stripe there are 2 plans: one where the DB `slug` matches the Stripe `ID` and one where the Stripe `ID` is suffixed with `-exempt`. This is to satisfy the Stripe requirement that credit cards must be present even if the total bill comes out to 0 (even after crediting with invoice items).