#= require ./authed-route

App.ContainersNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Container'

  renderTemplate: ->
    @render 'containers/new', outlet: 'content'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    pendingContainerData = App.get('pendingContainerData')
    if controller.get('runCreate') && pendingContainerData
      controller.setProperties
        selectedHostSlug: pendingContainerData.get('selectedHostSlug')
        name: pendingContainerData.get('name')
        selectedOption: pendingContainerData.get('selectedOption')
        selectedTemplate: pendingContainerData.get('selectedTemplate')
        envVars: pendingContainerData.get('envVars')
        portMappings: pendingContainerData.get('portMappings')
        privileged: pendingContainerData.get('privileged')
        publishAll: pendingContainerData.get('publishAll')
      App.set('pendingContainerData', null)
      controller.send('create', pendingContainerData.get('repository'))
      return

    unless controller.get 'saving'
      controller.clear()
    Mp.track Mp.EventDashboardViewedNewContainer

  beforeModel: (transition) ->
    @_super.apply this, arguments

  resetController: (controller) ->
    # Query params are sticky, so we explicitly reset 'runCreate' so that bad
    # things won't happen.
    controller.set('runCreate', false)
