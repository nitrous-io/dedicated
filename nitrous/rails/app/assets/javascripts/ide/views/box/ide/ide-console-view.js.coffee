App.IdeConsoleView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'templates/box/ide/console'
  elementId: 'ide_console_view'
  classNameBindings: ['controller.fullscreen:fullscreen', 'alwaysFullScreen:fullscreen']

  alwaysFullScreen: false

  didClickFullScreen: ->
    if mainMenuController = App.get 'router.ideMainMenuController'
      mainMenuController.viewToggleFullScreenConsole()

  didInsertElement: ->
    @_super()
    $(window).bind 'resize.ideConsoleView', _.bind(@resizeTabBarHeight, this)
    this.didClickNew()

  willDestroyElement: ->
    @_super()
    $(window).unbind 'resize.ideConsoleView'

  resizeTabBarHeight: (->
    Em.run.sync()
    Em.run.next this, ->
      if $header = @$ '> header'
        tabBarHeight = $header.outerHeight()
        $header.css 'margin-top', -tabBarHeight
        @$().css 'padding-top', tabBarHeight
  ).observes 'controller.length'
