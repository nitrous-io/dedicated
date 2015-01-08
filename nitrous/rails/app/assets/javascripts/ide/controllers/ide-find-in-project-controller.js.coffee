App.IdeFindInProjectController = Em.Controller.extend
  needs: ['index', 'ideEditor']

  findInProjectDialog: null
  termBinding: 'findInProjectDialog.term'
  locationBinding: 'findInProjectDialog.location'
  optionsBinding: 'findInProjectDialog.optionsString'

  invalidField: null

  reset: ->
    @set 'findInProjectDialog', null

  valid: ->
    @set 'invalidField', null
    if (@get('term') || '').trim() == ''
      @set 'invalidField', 'term'
      return false
    true

  showFindDialog: (location) ->
    @set 'findInProjectDialog', App.FindInProjectDialog.create location: location

  actions:
    find: ->
      if !@valid()
        App.postError "Invalid #{@get 'invalidField'}"
        return

      location = @get('location')
      location = '.' if !location
      App.util.mixpanel 'Started Find in Project',
        location: location
        options: @get('options')
      if socket = @get('controllers.index.opsSocket')
        socket.search @get('term'), location, @get('options'), (command, tabId) =>
          editorController = @get('controllers.ideEditor')
          doc = editorController.createTab({ term: @get('term'), path: location }, App.SearchResults)
          command.set "resultDocuments.#{tabId}", doc
      this.reset()
