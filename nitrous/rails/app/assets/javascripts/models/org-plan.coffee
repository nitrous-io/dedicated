#= require ./plan

App.OrgPlan = klass = App.Plan.extend
  orgSlug: null

klass.reopenClass
  url: '/orgs/:orgSlug/plans'
