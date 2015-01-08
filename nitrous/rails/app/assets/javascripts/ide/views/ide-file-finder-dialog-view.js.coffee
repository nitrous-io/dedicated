App.IdeFileFinderDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/file-finder-dialog'
  elementId: 'ide-file-finder-dialog-view'
  contentBinding: 'controller.fileFinderDialog'

  pollerKey: null

  didInsertElement: ->
    @get('controller').initialize()

  viewDidAppear: ->
    @_super()
    this.setupPoller()

  viewDidDisappear: ->
    @_super()
    if pollerKey = @get('pollerKey')
      Ember.run.cancel(pollerKey)

  click: (e) ->
    if $(e.target).is(@$())
      @hide()
    e.stopPropagation()

  keyDown: (e) ->
    suppress = ->
      e.preventDefault()
      e.stopPropagation()
    switch e.which
      when 27 # escape
        @hide()
      when 38 # up
        suppress()
        @get('content')?.decrementIndex()
      when 40 # down
        suppress()
        @get('content')?.incrementIndex()
      when 10, 13
        suppress()
        if filename = @get('content.selectedFile.filename')
          this.didClickResult(filename)

  setupPoller: ->
    @set 'pollerKey', Ember.run.later(this, (=>
      this.poller()
      this.setupPoller()
    ), 200)

  poller: ->
    nextSearch = @get('content.pattern')
    if nextSearch != @get('content.currentSearch')
      @get('controller').startFind nextSearch, @get('content.basePath')
      @set 'content.currentSearch', nextSearch

  didClickResult: (filename) ->
    # filename will be relative to home directory
    path = App.Path.create absolutePath: [ App.get('container.homePath'), filename ].join('/')
    if sidebar = @get 'controller.controllers.ideSidebar'
      sidebar.openPath path,
        done: =>
          this.hide()
        source: 'filefinder'
