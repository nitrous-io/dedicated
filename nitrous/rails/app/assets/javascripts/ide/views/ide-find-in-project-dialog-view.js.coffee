App.IdeFindInProjectDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/find-in-project-dialog'
  elementId: 'ide-find-in-project-dialog-view'
  contentBinding: 'controller.findInProjectDialog'

  keyPress: (e) ->
    if e.which == 13
      this.didClickFind()

  didClickFind: ->
    if @get('content').valid()
      @get('controller').startFind @get('content.term'),
        @get('content.location'), @get('content.optionsString')
    else
      false
