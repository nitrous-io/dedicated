App.IdeConsoleView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'ide/console'
  elementId: 'ide-console-view'
  classNameBindings: ['controller.fullscreen:fullscreen', 'controller.minimized:minimized']

  didInsertElement: ->
    @_super()
    @get('controller').createTab()

  actions:
    didClickMinimizeConsole: ->
      if mainMenuController = App.get 'router.ideMainMenuController'
        mainMenuController.windowMinimizeConsole()

    didClickRestoreConsole: ->
      if mainMenuController = App.get 'router.ideMainMenuController'
        mainMenuController.windowRestoreConsole()

    didClickFullScreen: ->
      if mainMenuController = App.get 'router.ideMainMenuController'
        mainMenuController.windowToggleFullScreenConsole()
