#= require ./authed-route

App.AppIndexRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'app/index', into: 'app'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName')
