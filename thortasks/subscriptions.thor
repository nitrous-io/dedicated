class Subscriptions < Thor
  desc "list [filter] IDENT", "perform action on a subscription for customer"
  def list(filter="all", ident)
    Subscription.list_subscriptions(filter, ident)
  end

  desc "info IDENT", "perform action on a subscription for customer"
  def info(ident)
    Subscription.info(ident)
  end

  desc "cancel IDENT", "perform action on a subscription for customer"
  def cancel(ident)
    Subscription.cancel(ident)
  end
end

# vim:ft=ruby
