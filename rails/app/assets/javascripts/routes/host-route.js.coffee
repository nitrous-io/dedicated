App.HostRoute = App.AuthedRoute.extend
  model: (params) ->
    fetch = App.Host.fetch(params.hostSlug)
    host = fetch.value
    App.set 'selectedHost', host
    fetch

  renderTemplate: ->
    @render 'host', outlet: 'content'

  deactivate: ->
    App.set 'selectedHost', null
