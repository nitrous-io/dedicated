App.IdeFindInProjectController = Em.Controller.extend
  needs: ['ide', 'ideEditor']

  findInProjectDialog: null

  reset: ->
    @set 'findInProjectDialog', null

  showFindDialog: (location) ->
    @set 'findInProjectDialog', App.FindInProjectDialog.create location: location

  startFind: (term, location, options) ->
    location = '.' if !location
    App.util.mixpanel 'Started Find in Project',
      location: location
      options: options
    if socket = @get('controllers.index.opsSocket')
      socket.search term, location, options, (command, tabId) =>
        editorController = @get('controllers.ideEditor')
        doc = editorController.createTab({ term: term, path: location }, App.SearchResults)
        command.set "resultDocuments.#{tabId}", doc
    this.reset()
