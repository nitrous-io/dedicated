#= require ./subscription

App.OrgSubscription = klass = App.Subscription.extend
  orgSlug: null

klass.reopenClass
  url: '/orgs/:orgSlug/subscriptions'
