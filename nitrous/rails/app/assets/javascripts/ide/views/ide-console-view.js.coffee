App.IdeConsoleView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'ide/templates/console'
  elementId: 'ide_console_view'
  classNameBindings: ['controller.fullscreen:fullscreen', 'alwaysFullScreen:fullscreen']

  alwaysFullScreen: false

  didClickFullScreen: ->
    if mainMenuController = App.get 'router.ideMainMenuController'
      mainMenuController.viewToggleFullScreenConsole()

  didInsertElement: ->
    @_super()
    this.didClickNew()
