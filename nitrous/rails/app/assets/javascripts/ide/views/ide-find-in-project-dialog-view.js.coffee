App.IdeFindInProjectDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/find-in-project-dialog'
  elementId: 'ide-find-in-project-dialog-view'
  contentBinding: 'controller.findInProjectDialog'

  termFieldInvalid: Em.computed.equal('controller.invalidField', 'term')

  keyDown: (e) ->
    suppress = ->
      e.preventDefault()
      e.stopPropagation()
    switch e.which
      when 13 # Enter
        suppress()
        @find()
      when 27 # Escape
        @hide()

  find: ->
    @get('controller').send 'find'

  actions:
    didClickFind: ->
      @find()
      false
