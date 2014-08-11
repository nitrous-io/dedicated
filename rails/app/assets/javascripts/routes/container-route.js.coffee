App.ContainerRoute = App.AuthedRoute.extend
  model: (params) ->
    record = App.Container.fetch(params.containerSlug).value
    App.set 'selectedContainer', record
    record

  renderTemplate: ->
    @render 'container', outlet: 'content'

  deactivate: ->
    App.set 'selectedContainer', null
