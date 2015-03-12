App.IdeGoToLineController = Em.Controller.extend
  needs: ['ide', 'ideEditor']

  goToLineDialog: null

  reset: ->
    @set 'goToLineDialog.hide', true
    @set 'goToLineDialog', null

  showGoToLineDialog: ->
    @set 'goToLineDialog', App.GoToLineDialog.create

  actions:
    goToLine: ->
      lineStr = @get('goToLineDialog').line
      if line = parseInt(lineStr)
        editorController = @get('controllers.ideEditor')
        editorController.get('activeTab')?.set('gotoLine', line)
