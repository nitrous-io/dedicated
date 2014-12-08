App.IdeConsoleView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'ide/console'
  elementId: 'ide-console-view'
  classNameBindings: ['controller.fullscreen:fullscreen']

  didClickFullScreen: ->
    if mainMenuController = App.get 'router.ideMainMenuController'
      mainMenuController.viewToggleFullScreenConsole()

  didInsertElement: ->
    @_super()
    this.didClickNew()
