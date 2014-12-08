App.IdeEditorView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'ide/templates/editor'
  elementId: 'ide_editor_view'
  classNameBindings: ['controller.fullscreen:fullscreen']

  didInsertElement: ->
    @_super()
    $(window).bind 'resize.ideEditorView', _.bind(@resizeTabBarHeight, this)
    this.resizeTabBarHeight()
    this._sidebarWidthDidChange()
    this._consoleHeightDidChange()

  willDestroyElement: ->
    @_super()
    $(window).unbind 'resize.ideEditorView'

  didClickFullScreen: ->
    if mainMenuController = App.get 'router.ideMainMenuController'
      mainMenuController.viewToggleFullScreenEditor()

  resizeTabBarHeight: (->
    Em.run.sync()
    Em.run.next this, ->
      if $header = @$ '> header'
        tabBarHeight = $header.outerHeight()
        $header.css 'margin-top', -tabBarHeight
        @$().css 'padding-top', tabBarHeight
  ).observes 'controller.@each.title'

  _consoleHeightDidChange: (->
    consoleHeight = App.get 'router.ideLayoutController.consoleHeight'
    @$().css 'paddingBottom', (consoleHeight + 4)
  ).observes 'App.router.ideLayoutController.consoleHeight'

  _sidebarWidthDidChange: (->
    paneWidth = App.get 'router.ideLayoutController.sidebarWidth'
    @$().css 'paddingLeft', (paneWidth + 4)
  ).observes 'App.router.ideLayoutController.sidebarWidth'
