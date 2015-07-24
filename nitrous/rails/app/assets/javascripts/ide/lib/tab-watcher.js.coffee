class @TabWatcher
  instance = null
  constructor: (@rootPane, @container) ->
    @changedAt = null
    @lastSynced = null
    instance ?= this

  notifyOfChange: ->
    @scheduleSync()

  scheduleSync: (delay = 25000)->
    return if @timer
    @timer = window.setTimeout(=>
      @timer = null
      @syncChanges()
    , delay)

  syncChanges: ->
    allTabs = @rootPane.allTabs()
    content = allTabs.map (tab)->
      pathValue = null
      path = tab.get('path')
      if typeof path is 'string'
        pathValue = path
      else
        pathValue = path.get('absolutePath')

      {
        path: pathValue,
        scrollPosition: tab.get('scrollPosition'),
        selections: tab.get('selections'),
        active: tab.get('active'),
        readme: tab.get('readme')
      }

    @container.setUserData('openTabs', content).fail =>
      @scheduleSync(10000)

  @notifyOfChange: ->
    return unless instance
    instance.notifyOfChange()

  @syncChanges: ->
    return unless instance
    instance.syncChanges()