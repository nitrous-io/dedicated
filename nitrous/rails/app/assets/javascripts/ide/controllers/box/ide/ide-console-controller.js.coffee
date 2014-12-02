App.IdeConsoleController = Em.ArrayController.extend App.IdeTabControllerMixin,
  needs: ['ideFileFinder']

  modelClass: App.ConsoleSession
  content: null
  fullscreen: false

  initialize: ->
    @set 'content', []
    @set 'fullscreen', (App.localStorage.get('fullscreenConsole') ? false)

  fullscreenDidChange: (->
    App.localStorage.set 'fullscreenConsole', @get('fullscreen')
  ).observes 'fullscreen'

  createTab: (attrs={}) ->
    App.util.mixpanelIncrement 'ide_new_console_tab_count'
    @_super(attrs)

  reset: ->
    @set 'content', null
