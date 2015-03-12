App.IdeGoToLineDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/go-to-line-dialog'
  elementId: 'ide-go-to-line-dialog-view'
  contentBinding: 'controller.goToLineDialog'

  hide: ->
    @set('content.line', null)
    @_super()

  keyDown: (e) ->
    switch e.which
      when 13 # Enter
        e.preventDefault()
        e.stopPropagation()
        @goToLine()
      when 27 # Escape
        @hide()

  goToLine: ->
    @get('controller').send 'goToLine'
    @hide()
