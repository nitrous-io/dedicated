App.IdeFindInProjectDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/box/ide/find-in-project-dialog'
  elementId: 'ide_find_in_project_dialog_view'
  contentBinding: 'controller.findInProjectDialog'

  init: ->
    @_super.apply this, arguments
    @set 'focusSelector', '#search_term'

  viewDidAppear: ->
    @_super()

  keyPress: (e) ->
    if e.which == 13
      this.didClickFind()

  didClickFind: ->
    if @get('content').valid()
      @get('controller').startFind @get('content.term'),
        @get('content.location'), @get('content.optionsString')
    else
      return false
