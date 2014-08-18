App.HostRoute = App.AuthedRoute.extend
  model: (params) ->
    fetch = App.Host.fetch(params.hostSlug)
    App.set 'selectedHost', fetch.value
    fetch

  renderTemplate: ->
    @render 'host', outlet: 'content'

  deactivate: ->
    App.set 'selectedHost', null
