#= require ./authed-route

App.ContainerRoute = App.AuthedRoute.extend
  model: (params) ->
    fetch = App.Container.fetch(params.containerSlug)
    container = fetch.value
    App.set 'selectedContainer', container
    fetch.then =>
      container.loadConfig()
    , (jqxhr) =>
      if jqxhr.status == 404
        Em.run.next ->
          App.snackbar.post("Container “#{params.containerSlug}” could not be found.")
          App.get('containers')?.reload()
      @transitionTo 'home.index'
    container

  renderTemplate: ->
    @render 'container', outlet: 'content'

  deactivate: ->
    App.set 'selectedContainer', null
