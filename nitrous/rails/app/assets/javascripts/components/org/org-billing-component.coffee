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

  actions:
    updateCard: ->
      PubSub.publish 'org.billing.wantToAddCard'
