#= require ./authed-route

App.HostsNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Workspace'

  renderTemplate: ->
    @render 'hosts/new', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
    if App.get('pendingHost')
      controller.set('host', App.get('pendingHost'))
      App.set('pendingHost', null)
      controller.send 'create'
