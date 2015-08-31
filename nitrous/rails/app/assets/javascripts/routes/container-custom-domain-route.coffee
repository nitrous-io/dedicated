#= require ./authed-route

App.ContainerCustomDomainRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/custom-domain', into: 'container'

  afterModel: (container, transition) ->
    @_super.apply this, arguments

    if !container.get('paidFeaturesEnabled')
      transition.abort()
      App.snackbar.post("Custom domain is not available for your container")
      @transitionTo 'container.index', container.get 'slug'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' › Custom Domain'
    Mp.track Mp.EventDashboardViewedContainerCustomDomain
