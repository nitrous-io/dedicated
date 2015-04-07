#= require ./authed-route

App.ContainerRoute = App.AuthedRoute.extend
  model: (params) ->
    isShared = App.get('sharedContainers').findBy('slug', params.containerSlug)
    model = if isShared
      App.SharedContainer
    else
      App.Container
    fetch = model.fetch(params.containerSlug)
    container = fetch.value
    App.set 'selectedContainer', container
    fetch.catch (jqxhr) =>
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
