#= require ./authed-route

App.HostsNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Workspace'
    App.get('plans')?.reload()

  renderTemplate: ->
    @render 'hosts/new', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    pendingHost = App.get('pendingHost')
    if pendingHost && !App.get('newUser')
      controller.set('host', pendingHost)
      App.set('pendingHost', null)
      controller.send 'create', pendingHost
      return

    unless controller.get 'saving'
      controller.clear()
    Mp.track Mp.EventDashboardViewedNewHost
