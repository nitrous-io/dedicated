#= require ./billing-profile

App.OrgBillingProfile = klass = App.BillingProfile.extend
  orgSlug: null

klass.reopenClass
  url: '/orgs/:orgSlug/billing_profile'
