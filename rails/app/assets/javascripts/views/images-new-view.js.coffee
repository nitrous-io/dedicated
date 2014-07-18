App.ImagesNewView = Em.View.extend
  elementId: 'images-new-view'
  classNames: ['ui', 'card']

  codeMirror: null

  didInsertElement: ->
    @_super()
    @set 'codeMirror', CodeMirror.fromTextArea @$('#dockerfile-code')[0],
      lineNumbers: true
      viewportMargin: Infinity

  willDestroyElement: ->
    @_super()
    codeMirror = @get 'codeMirror'
    codeMirror.toTextArea()
    @set 'codeMirror', null
