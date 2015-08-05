App.OrgFrameComponent = Em.Component.extend
  layoutName: 'components/org/frame'
  elementId: 'org-frame'
  classNames: ['ui', 'card']

  model: null
  currentTab: null

  membersSelected: Em.computed.equal 'currentTab', 'members'
  billingSelected: Em.computed.equal 'currentTab', 'billing'

  didInsertElement: ->
    @_super()
    @$('.ui.tab-bar li.disabled > a').on 'click.orgs-frame', ->
      false

  willDestroyElement: ->
    @_super()
    @$('.ui.tab-bar li.disabled > a').off 'click.orgs-frame'
