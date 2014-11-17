#= require ./authed-route

App.HostsNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Application'

  renderTemplate: ->
    @render 'hosts/new', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
