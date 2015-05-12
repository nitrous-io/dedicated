#= require ./authed-route

App.ContainerRoute = App.AuthedRoute.extend
  model: (params) ->
    # we wait for sharedContainers to be ready in case a user is requesting
    # a route for a shared container (which we will have internally)
    # which would return a different class
    if App.get('sharedContainers.loaded') && App.get('containers.loaded')
      return @_getModel(params)
    else
      waiting = ['sharedContainers', 'containers'].map (s) =>
        return if App.get(s).get("loaded")
        new Ember.RSVP.Promise (resolve) =>
          App.get(s).one 'loaded', =>
            resolve()
          App.get(s).one 'failed', =>
            resolve()

      new Ember.RSVP.Promise (resolve) =>
        Ember.RSVP.all(waiting).then =>
          resolve(@_getModel(params))

  renderTemplate: ->
    @render 'container', outlet: 'content'

  deactivate: ->
    App.set 'selectedContainer', null

  _getModel: (params) ->
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
          App.fetchContainers()
      @transitionTo 'home.index'
    container
