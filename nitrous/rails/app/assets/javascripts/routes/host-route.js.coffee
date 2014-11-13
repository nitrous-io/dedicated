App.HostRoute = App.AuthedRoute.extend
  model: (params) ->
    fetch = App.Host.fetch(params.hostSlug)
    host = fetch.value
    App.set 'selectedHost', host
    fetch.catch (jqxhr) =>
      if jqxhr.status == 404
        Em.run.next ->
          App.snackbar.post("Application “#{params.hostSlug}” could not be found.")
          App.get('hosts')?.reload()
      @transitionTo 'home.index'
    host

  renderTemplate: ->
    @render 'host', outlet: 'content'

  deactivate: ->
    App.set 'selectedHost', null
