App.IdeFindInProjectController = Em.Controller.extend
  needs: ['ide', 'ideEditor']

  opsSocketBinding: Em.Binding.oneWay 'controllers.ide.opsSocket'
  findInProjectDialog: null
  termBinding: Ember.Binding.oneWay('findInProjectDialog.term')
  locationBinding: Ember.Binding.oneWay('findInProjectDialog.location')
  optionsBinding: Ember.Binding.oneWay('findInProjectDialog.optionsString')

  invalidField: null

  reset: ->
    @set 'findInProjectDialog.hide', true
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

      term = @get('term')
      location = @get('location') || App.get('container.homePath')
      options = @get('options')

      socket = @get('opsSocket')
      editorController = @get('controllers.ideEditor')
      tab = editorController.createTab({ term: term, path: location }, App.SearchResults)

      # Generate a unique search ID so that we can handle concurrent searches.
      searchId = Date.now().toString()

      onOutput = (err, data) ->
        return unless data?.id == searchId
        if err
          removeHandlers()
          return
        if data.results?.length
          tab.addSearchResults(data.results)

      onError = (err, data) ->
        return unless data?.id == searchId
        removeHandlers()
        tab.addError(err || data.error)

      onCompleted = (err, data) ->
        return unless data?.id == searchId
        removeHandlers()
        tab.doComplete()

      removeHandlers = ->
        socket.off 'search.output', onOutput
        socket.off 'search.error', onError
        socket.off 'search.completed', onCompleted

      socket.on 'search.output', onOutput
      socket.on 'search.error', onError
      socket.on 'search.completed', onCompleted

      socket.search searchId, term, location, options, (err) =>
        if err
          App.postError err
          return
        this.reset()

      App.util.mixpanel 'Started Find in Project',
        location: location
        options: options
