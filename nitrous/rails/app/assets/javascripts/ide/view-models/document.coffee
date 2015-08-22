#= require ./tab-content

LINE_ERROR_MATCH = /\.go:(\d+):(\d+):?\s+(.+)$/

untitledCount = 0

App.Document = App.TabContent.extend
  isDocumentTab: true
  contentType: 'text'

  plugin: null
  editorMode: 'text/plain'

  otClient: null
  otUndoManagerOverride: null

  findTrigger: null
  findNextTrigger: null
  findPrevTrigger: null
  findError: null

  replaceTrigger: null
  replaceAllTrigger: null
  selectNextTrigger: null

  foldCodeTrigger: null
  unfoldCodeTrigger: null
  toggleCommentTrigger: null
  splitSelectionIntoLinesTrigger: null
  removeTrailingWhitespaceTrigger: null

  goToLine: null
  selections: null
  scrollPosition: null

  errorCountBinding: 'lineWidgets.length'

  currentOffset: null

  readOnly: false

  lineWidgets: Em.A([])

  init: ->
    @_super.apply this, arguments

    @setPluginManager()

  setPluginManager: ->
    if pl = (App.CodePluginsManager.forType @get('editorMode'))
      @set 'plugin', new pl(@)

  updatePluginManager: (->
    @setPluginManager()
  ).observes 'editorMode'

  line: (->
    (@get('selections.0.head.line') || 0) + 1
  ).property 'selections'

  column: (->
    (@get('selections.0.head.ch') || 0) + 1
  ).property 'selections'

  title: (->
    @get('path.name') || "Untitled #{untitledCount += 1}"
  ).property 'path.name'

  processErrMessage: (stderr)->
    errors = Em.A([])
    if stderr
      messages = stderr.trim().split('\n')
      for line in messages
        match = line.match(LINE_ERROR_MATCH)
        if match
          msg = document.createElement('div')
          excl = document.createElement('i')
          excl.className = "fa fa-exclamation-circle"
          msg.appendChild(excl)
          msg.appendChild(document.createTextNode(" " + match[3]))
          msg.className = 'line-error-message'
          errors.push({line: parseInt(match[1])-1, node: msg, options: {coverGutter: false}})
    @set 'lineWidgets', errors

  supportsGoToDefinition: (->
    @get('plugin')?.supportsGoToDefinition || false
  ).property 'plugin'
