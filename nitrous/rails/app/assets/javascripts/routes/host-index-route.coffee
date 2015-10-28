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

  afterModel: (model, transition) ->
   @_super.apply this, arguments

   # Block route to host with canceled subscription. Context: On canceling
   # trial subscription, which is immediate, user should not be able to
   # see workspace dashboard. If subscription is canceled the subscription data
   # will not be serialized. 
   if !model.get('subscription')?
     transition.abort()
     App.snackbar.post("Workspace has been terminated")
     @transitionTo 'home.index'
