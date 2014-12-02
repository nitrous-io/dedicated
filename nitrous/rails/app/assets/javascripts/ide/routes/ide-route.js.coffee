message = 'You will lose unsaved work if you close the IDE.'

App.IdeRoute = Em.Route.extend
  activate: ->
    # warn the user before actually unload the window. This is helpful to
    # prevent mis-click close tab / cmd + w
    $(window).bind 'beforeunload.ide', -> message

  deactivate: ->
    $(window).unbind 'beforeunload.ide'

  actions:
    willTransition: (transition) ->
      return true if App.get 'isChromeWebView'
      return true if window.confirm(message + '\n\nAre you sure you want to leave this page?')
      url = transition.router.recognizer.generate 'box.ide', box_id: this.get('controller.content.id')
      transition.router.updateURL(url)
      transition.abort()
