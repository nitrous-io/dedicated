# Promocodes

Promocodes allow users to apply a promocode during signup or subscription creation to unlock functionality such as extended trial periods. It is not intended to replace Stripe coupons but instead provide functionality that Stripe coupons do not provide. If Stripe coupons do provide such a functionality we should make use of it, using promocodes as a proxy.

## Current implementation

The only redeemable feature currently is `trial_seconds`. When not `NULL` it allows for a subscription's trial to expire after the configured days. Originally this may have been part of the internal plan or Stripe plan but now we can provide custom trial periods for certain use cases.

Promocodes are created in `promo_codes` and then join to specific plans as specified in `promo_code_plans`. This allows promo code functionality such as the `trial_seconds` to be only applicable to certain plans.


### `promo_code_uses`

`promo_code_uses` records the usage of promo codes by users (+ their subscriptions) to allow for the control of `quantity`. Instead of locking up a `promo_code_row` to do a subtraction on the `quantity` column, instead a `COUNT` is performed against uses.

`promo_code_uses` also allows us to preemptively assign a use of a promo code to a user before they create a host or create a subscription (in the case of orgs). If the `subscription_id` is `NULL` then it is in the "pending" state can can be claimed when found with `FindPendingClaim`. A `promo_code_uses` is claimed when `subscription_id` is set, which is almost immediately after subscribing to a Stripe subscription.

* `queue_handler` uses this because the subscription is actually created there. So in the `controllers/hosts/create.go` we allocate a promo code if possible and then let `queue_handler` pick up the pending.
* `controllers/subscriptions/subscriptions.go#Subscribe` uses this if no `promo_code` param is provided then it will try to find any "pending" promo code uses for the user for that subscribing plan. If found then it is claimed.

### Signup

If a user signs up by providing a `promo_code` in the request then the usage of a promo code is stored in `promo_code_uses`. Because the `subscription_id` column is `NULL` it indicates this `promo_code` is available for consumption when the next new `subscription` is created. Our subscription process will look for the first available "pending" `PromoCodeUse` using `models/promocode/promocode.go#FindPendingClaim` for the given user ID and plan ID. Once the subscription succeeds the `subscription_id` column is filed out and that use is no longer "pending".

### Plan subscription

As mentioned in **promo_code_uses** the `controllers/subscriptions/subscriptions.go#Subscribe` endpoint will take a `promo_code` to make a claim against or it will find any pending claims. If `promo_code` is provided by it is not available for the plan or does not exist then an error is returned.

Once the subscription is successfully performed then the `promo_code_uses` is updated with the `subscription_id`. If the subscription fails then the `promo_code_use` is deleted (freeing up the `quantity` of `promo_codes`) **only** if the `promo_code_use` was created in the request (unaffecting users who had "pending" promocodes).

## Future implementation

The `NULL`able nature of `trial_seconds` allows for new columns to be added to provide different features of promo codes. For example we may want to use a special Stripe coupon for a promocode which could be another `NULL`able column OR we may want to unlock a certain range of plans for a promocode.
