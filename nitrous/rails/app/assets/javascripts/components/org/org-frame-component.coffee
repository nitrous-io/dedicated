#= require ./org-members-mixin

App.OrgFrameComponent = Em.Component.extend App.OrgMembersMixin,
  layoutName: 'components/org/frame'
  elementId: 'org-frame'
  classNames: ['ui', 'card']

  org: null
  currentTab: null

  membersSelected: Em.computed.equal 'currentTab', 'members'
  billingSelected: Em.computed.equal 'currentTab', 'billing'
  addCardSelected: Em.computed.equal 'currentTab', 'add-card'

  billingOrAddCardSelected: Em.computed.or 'billingSelected', 'addCardSelected'

  canSeeBillingTab: (->
    ['owner', 'admin'].contains(@get 'user.role')
  ).property 'user.role'

  didInsertElement: ->
    @_super()
    @$('.ui.tab-bar li.disabled > a').on 'click.orgs-frame', ->
      false

  willDestroyElement: ->
    @_super()
    @$('.ui.tab-bar li.disabled > a').off 'click.orgs-frame'
