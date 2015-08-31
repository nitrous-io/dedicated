App.TopnavView = Em.View.extend
  elementId: 'topnav-view'
  classNames: ['ui', 'topnav', 'page-header']

  newHostTitle: (->
    if @get('canCreateHosts')
      maxHosts = App.get('userProfile.maxHosts')
      "You can only create #{maxHosts} workspace#{if maxHosts != 1 then "s" else ""}."
  ).property 'canCreateHosts', 'userProfile.maxHosts'

  hideCreateHost: (->
    return false if @get('controller')?.controllerFor('orgs')?.get('collection.length')

    App.get('billingProfile.free')
  ).property 'App.billingProfile.free'

  canCreateHosts: (->
    return true if @get('controller')?.controllerFor('orgs')?.get('collection.length')

    if App.get('billingProfile.isNoCardState')
      return true

    if App.get('billingProfile.free') && (@get('hosts.length') > 0)
      return false

    if !App.get('billingProfile.active')
      return false

    if maxHosts = App.get('userProfile.maxHosts')
      return App.get('hosts.loaded') && (App.get('hosts.length') < maxHosts)

    return true
  ).property('controller.controllers.orgs.collection.length', 'App.billingProfile.isNoCardState', 'App.billingProfile.free', 'App.billingProfile.active',
    'App.userProfile.maxHosts', 'App.hosts.loaded', 'App.hosts.length')

  didInsertElement: ->
    @_super()

  actions:
    sendFeedback: ->
      App.trigger 'open-feedback-dialog'

    switchUser: ->
      App.trigger 'open-switch-user-dialog'

    creditUser: ->
      App.trigger 'open-credit-user-dialog'

    viewUser: ->
      App.trigger 'open-view-user-dialog'

    updateAdminHosts: ->
      App.trigger 'show-update-hosts', App.get('hosts')

    switchBackAdmin: ->
      App.switchBackAdmin()
