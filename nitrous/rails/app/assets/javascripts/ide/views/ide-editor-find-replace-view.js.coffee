App.IdeEditorFindReplaceView = klass = Em.View.extend
  templateName: 'ide/editor-find-replace'
  classNames: ['ide-editor-find-replace-view']
  classNameBindings: ['hidden']

  document: null

  hidden: true

  useRegex: false
  matchCase: false
  showReplace: false

  searchText: ''
  replacementText: ''

  editorBinding: Em.Binding.oneWay 'parentView.editor'

  documentFindTriggerDidChange: (->
    if @get 'document.findTrigger'
      @setProperties
        showReplace: false
        hidden: false
      @set 'document.findTrigger', false
      @focus()
    @refreshEditor()
  ).observes 'document.findTrigger'

  documentFindNextTriggerDidChange: (->
    if @get 'document.findNextTrigger'
      @setProperties
        showReplace: false
        hidden: false
      @set 'document.findNextTrigger', false
    @refreshEditor()
    @focus()
    @find(false)
  ).observes 'document.findNextTrigger'

  documentFindPrevTriggerDidChange: (->
    if @get 'document.findPrevTrigger'
      @setProperties
        showReplace: false
        hidden: false
      @set 'document.findPrevTrigger', false
    @refreshEditor()
    @focus()
    @find(true)
  ).observes 'document.findPrevTrigger'

  documentReplaceTriggerDidChange: (->
    if @get 'document.replaceTrigger'
      @setProperties
        showReplace: true
        hidden: false
      @set 'document.replaceTrigger', false
      @focus()
    @refreshEditor()
  ).observes 'document.replaceTrigger'

  focus: ->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('.search-text').select().focus()

  refreshEditor: ->
    Em.run.scheduleOnce 'afterRender', =>
      @get('editor')?.refresh()

  find: (reverse) ->
    matchCase = @get 'matchCase'
    useRegex = @get 'useRegex'
    if query = @get 'searchText'
      @get('editor')?.find(query, reverse, matchCase, useRegex)

  replace: (all) ->
    matchCase = @get 'matchCase'
    useRegex = @get 'useRegex'
    text = @get 'replacementText'
    if query = @get 'searchText'
      @get('editor')?.replace(query, text, all, matchCase, useRegex)

  actions:
    close: ->
      @set 'hidden', true
      return unless editor = @get('editor')
      editor.clearSearch()
      editor.focus()

    findNext: ->
      @find(false)

    findPrev: ->
      @find(true)

    replace: ->
      @replace(false)

    replaceAll: ->
      @replace(true)

App.IdeEditorFindReplaceTextField = klass = Em.TextField.extend
  insertNewline: (evt) ->
    if parentView = @get 'parentView'
      if evt.shiftKey
        parentView.send('findPrev')
      else
        parentView.send('findNext')

  keyDown: (evt) ->
    parentView = @get 'parentView'
    if parentView && evt && evt.keyCode == 27
      parentView.send('close')
    true
