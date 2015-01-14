message = 'You will lose unsaved work if you close the IDE.'

App.IdeRoute = Em.Route.extend
  beforeModel: ->
    if App.get 'token'
      App.CurrentUser.fetch().then (user) ->
        App.set 'currentUser', user
    else
      # redirect to login screen if user hasn't logged in
      window.location.href = window.location.href.replace(window.location.pathname, '/app#login')

  model: (params) ->
    @_super.apply this, arguments
    slug = params.containerSlug
    App.set 'slug', slug

    notFoundPath = window.location.href.replace(window.location.pathname, '/404')
    if slug
      App.Container.fetch(slug).then (container) ->
        App.set 'container', container
        container
      , -> # failure
        window.location.href = notFoundPath
    else
      window.location.href = notFoundPath

  afterModel: (container, transition)->
    container?.loadHost().catch ->
      transition.abort()

  activate: ->
    # warn the user before actually unload the window. This is helpful to
    # prevent mis-click close tab / cmd + w
    $(window).bind 'beforeunload.ide', -> message

  deactivate: ->
    $(window).unbind 'beforeunload.ide'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.initialize()

  actions:
    willTransition: (transition) ->
      return true if App.get 'isChromeWebView'
      return true if window.confirm(message + '\n\nAre you sure you want to leave this page?')
      url = transition.router.recognizer.generate 'box.ide', box_id: this.get('controller.content.id')
      transition.router.updateURL(url)
      transition.abort()
