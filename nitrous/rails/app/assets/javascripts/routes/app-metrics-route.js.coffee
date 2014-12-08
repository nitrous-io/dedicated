#= require ./authed-route

App.AppMetricsRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'app/metrics', into: 'app'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' › Metrics'
