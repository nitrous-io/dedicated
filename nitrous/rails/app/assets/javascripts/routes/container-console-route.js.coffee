#= require ./authed-route

message = 'Leaving this page will close the terminal session and terminate your current process'

App.ContainerConsoleRoute = App.AuthedRoute.extend
  deactivate: ->
    @get('controller.socket')?.disconnect()
    @set 'controller.model', null

  renderTemplate: ->
    @render 'container/console', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' › Console'

  activate: ->
    # warn the user before actually unload the window
    $(window).bind 'beforeunload.ide', -> message

  deactivate: ->
    $(window).unbind 'beforeunload.ide'

  actions:
    willTransition: (transition) ->
      unless window.confirm(message)
        transition.abort()
