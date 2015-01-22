App.IdeConsoleView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'ide/console'
  elementId: 'ide-console-view'
  classNameBindings: ['controller.fullscreen:fullscreen', 'controller.minimized:minimized', 'App.editorFullscreen:hide']

  didInsertElement: ->
    @_super()
    @didClickNew()

  actions:
    didClickMinimizeConsole: ->
      if mainMenuController = App.get 'router.ideMainMenuController'
        mainMenuController.viewMinimizeConsole()

    didClickMaximizeConsole: ->
      if mainMenuController = App.get 'router.ideMainMenuController'
        mainMenuController.viewMaximizeConsole()

    didClickFullScreen: ->
      if mainMenuController = App.get 'router.ideMainMenuController'
        mainMenuController.viewToggleFullScreenConsole()
