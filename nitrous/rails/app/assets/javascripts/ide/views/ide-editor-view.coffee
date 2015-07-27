App.IdeEditorView = Em.View.extend
  templateName: 'ide/editor'
  elementId: 'ide-editor-view'
  classNameBindings: ['controller.fullscreen:fullscreen', 'App.consoleFullscreen:hide']
