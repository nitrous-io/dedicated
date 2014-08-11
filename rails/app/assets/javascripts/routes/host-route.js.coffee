App.HostRoute = App.AuthedRoute.extend
  model: (params) ->
    record = App.Host.fetch(params.hostSlug).value
    App.set 'selectedHost', record
    record

  renderTemplate: ->
    @render 'host', outlet: 'content'

  deactivate: ->
    App.set 'selectedHost', null
