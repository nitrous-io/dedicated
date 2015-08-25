App.IdeEditorView = Em.View.extend
  templateName: 'ide/editor'
  elementId: 'ide-editor-view'
  classNameBindings: ['controller.fullscreen:fullscreen', 'App.consoleFullscreen:hide']

  didInsertElement: ->
    @_super()
    $(window).on "focus.#{@get('elementId')}", =>
      @set('controller.activeTab.focused', true)

  willDestroyElement: ->
    @_super()
    $(window).off "focus.#{@get('elementId')}"