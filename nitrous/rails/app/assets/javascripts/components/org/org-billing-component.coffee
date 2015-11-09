#= require ./org-members-mixin

App.OrgBillingComponent = Em.Component.extend App.OrgMembersMixin,
  layoutName: 'components/org/billing'
  elementId: 'org-billing'

  org: null

  noBillingProfile: Em.computed.empty 'org.billingProfile'

  user: (->
    userId = App.get 'user.id'
    @get('org.members')?.findBy('userId', userId)
  ).property 'org.members.@each', 'App.user.id'

  cardUpdateAllowed: Em.computed.equal 'user.role', 'owner'

  didInsertElement: ->
    @_super()
    @fetchMembers()
    @reloadSubscriptions()
    @reloadInvoices()

  reloadSubscriptions: (->
    subscriptions = @get 'org.subscriptions'
    if subscriptions && subscriptions.get('loaded')
      subscriptions.reload()
  ).observes 'org'

  reloadInvoices: (->
    invoices = @get 'org.invoices'
    if invoices && invoices.get('loaded')
      invoices.reload()
  ).observes 'org'

  actions:
    updateCard: ->
      PubSub.publish 'org.billing.wantToAddCard'
