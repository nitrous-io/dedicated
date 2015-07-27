#= require ./authed-route

App.HostIndexRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/index', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName')
    model.prefetchResources()
    model.fetchStats()
    Mp.track Mp.EventDashboardViewedHost

  resetController: (controller) ->
    # Explicitly reset query params so that bad things won't happen.
    controller.set('justUpgraded', false)
