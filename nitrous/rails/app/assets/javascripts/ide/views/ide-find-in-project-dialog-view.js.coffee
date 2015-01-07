App.IdeFindInProjectDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/find-in-project-dialog'
  elementId: 'ide-find-in-project-dialog-view'
  contentBinding: 'controller.findInProjectDialog'

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
    if @get('content').valid()
      @get('controller').startFind(
        @get('content.term'),
        @get('content.location'),
        @get('content.optionsString')
      )

  actions:
    didClickFind: ->
      @find()
      false
