App.IdeCloneFromGitHubView = App.IdeModalDialogView.extend
  templateName: 'ide/clone-from-github-dialog'
  elementId: 'ide-clone-from-github-dialog-view'
  contentBinding: 'controller.cloneFromGitHubDialog'
  selectedRepositoryBinding: 'controller.selectedRepository'

  show: ->
    # It only triggers `keyDown` event when something is focused in the dialog
    # This is to make Escape works
    @$('select').focus()
    @_super()

  hide: ->
    @set('selectedRepository', null)
    @_super()

  click: (e) ->
    if $(e.target).is(@$())
      @hide()
    e.stopPropagation()

  selectedRepositoryChanged: ( ->
    if @get('selectedRepository')
      @get('controller').openTerminalAndCloneRepo(@get('selectedRepository'))
      @hide()
  ).observes('selectedRepository')

  keyDown: (e) ->
    switch e.which
      when 27 # Escape
        @hide()

  viewDidDisappear: ->
    @_super()
