App.IdeEditorView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'ide/editor'
  elementId: 'ide-editor-view'
  classNameBindings: ['controller.fullscreen:fullscreen']

  actions:
    didClickFullScreen: ->
      if mainMenuController = App.get 'router.ideMainMenuController'
        mainMenuController.viewToggleFullScreenEditor()
