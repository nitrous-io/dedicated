App.IdeEditorView = Em.View.extend App.IdeTabViewMixin,
  templateName: 'ide/templates/editor'
  elementId: 'ide_editor_view'
  classNameBindings: ['controller.fullscreen:fullscreen']

  didClickFullScreen: ->
    if mainMenuController = App.get 'router.ideMainMenuController'
      mainMenuController.viewToggleFullScreenEditor()
