App.IdeConsoleView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'ide/console'
  elementId: 'ide-console-view'
  classNameBindings: ['controller.fullscreen:fullscreen', 'App.editorFullscreen:hide']

  didInsertElement: ->
    @_super()
    @didClickNew()

  actions:
    didClickFullScreen: ->
      if mainMenuController = App.get 'router.ideMainMenuController'
        mainMenuController.viewToggleFullScreenConsole()
