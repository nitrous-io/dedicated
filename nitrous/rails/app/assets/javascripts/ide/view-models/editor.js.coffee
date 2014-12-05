App.Editor = klass = Em.Object.extend
  domTarget: null
  codeMirror: null

  text: null

  autoCloseBrackets: null
  highlightLine: null
  keybindings: null
  matchBrackets: null
  mode: null
  rulerColumn: null
  showLineNumbers: null
  showTrailingSpace: null
  softTabs: null
  tabSize: null
  theme: null
  wrapLines: null

  foldGutterClass: 'CodeMirror-foldgutter'

  macExtraKeys:
    'Tab': 'smartTab'
    'Ctrl-Space': 'autocomplete'
    'Cmd-/': 'toggleComment'
    'Cmd-Enter': 'insertLineAfter'
    'Shift-Cmd-Enter': 'insertLineBefore'
    'Ctrl-M': 'goMatchingBracket'
    'Cmd-D': 'selectNext'
  pcExtraKeys:
    'Tab': 'smartTab'
    'Ctrl-Space': 'autocomplete'
    'Ctrl-/': 'toggleComment'
    'Ctrl-Enter': 'insertLineAfter'
    'Shift-Ctrl-Enter': 'insertLineBefore'
    'Ctrl-M': 'goMatchingBracket'
    'Ctrl-D': 'selectNext'

  # keybindings in codemirror default keymap that we use in menu
  macOverrideKeys: ['Cmd-D']
  pcOverrideKeys: ['Shift-Ctrl-F', 'Ctrl-D']

  lintableMode: (->
    _.any CodeMirror.MODES_WITH_LINTING, (m) =>
      m == @get('mode')
  ).property 'mode'

  init: ->
    @_bootstrapCodeMirror()
    @_super.apply this, arguments

  destroy: ->
    if cm = @get('codeMirror')
      cm.off 'change'
      cm.off 'focus'
      cm.off 'blur'
      cm.off 'gutterClick'

  on: ->
    if cm = @get 'codeMirror'
      cm.on.apply cm, arguments

  isClean: ->
    if cm = @get('codeMirror')
      cm.isClean()

  markClean: ->
    if cm = @get('codeMirror')
      cm.markClean()

  refresh: ->
    if cm = @get('codeMirror')
      cm.refresh()

  focus: ->
    if cm = @get('codeMirror')
      cm.focus()

  find: ->
    if cm = @get 'codeMirror'
      cm.execCommand('find')

  findNext: ->
    if cm = @get 'codeMirror'
      cm.execCommand('findNext')

  selectNext: ->
    if cm = @get 'codeMirror'
      cm.execCommand('selectNextOccurrence')
      cm.focus()

  findPrev: ->
    if cm = @get 'codeMirror'
      cm.execCommand('findPrev')

  replace: ->
    if cm = @get 'codeMirror'
      cm.execCommand('replace')

  replaceAll: ->
    if cm = @get 'codeMirror'
      cm.execCommand('replaceAll')

  fold: ->
    if cm = @get 'codeMirror'
      cm.execCommand('fold')

  unfold: ->
    if cm = @get 'codeMirror'
      cm.execCommand('unfold')

  gotoLine: (line) ->
    if cm = @get 'codeMirror'
      cm.setCursor(parseInt(line) - 1, 0, scroll: true)
      cm.scrollIntoView() # above {scroll: true} option does not work as advertised

  toggleComment: ->
    if cm = @get 'codeMirror'
      cm.execCommand('toggleComment')

  splitSelectionIntoLines: ->
    if cm = @get 'codeMirror'
      cm.execCommand('splitSelectionByLine')

  modeDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'mode', @get 'mode'
      cm.setOption 'lint', @get 'lintableMode'
  ).observes 'mode'

  tabSizeDidChange: (->
    if cm = @get 'codeMirror'
      tabSize = @get 'tabSize'
      cm.setOption 'tabSize', tabSize
      cm.setOption 'indentUnit', tabSize
  ).observes 'tabSize'

  softTabsDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'indentWithTabs', !@get 'softTabs'
  ).observes 'softTabs'

  themeDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'theme', @get('theme')
  ).observes 'theme'

  wrapLinesDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'lineWrapping', @get('wrapLines')
  ).observes 'wrapLines'

  highlightLineDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'styleActiveLine', @get('highlightLine')
  ).observes 'highlightLine'

  showLineNumbersDidChange: (->
    if cm = @get 'codeMirror'
      if @get('showLineNumbers')
        # Reset folding gutters options otherwise the fold gutter will appear
        # to the left of the line number gutter.
        cm.operation =>
          cm.setOption 'lineNumbers', true
          cm.setOption 'foldGutter', cm.getOption('foldGutter')
          cm.setOption 'foldOptions', cm.getOption('foldOptions')
          cm.setOption 'gutters', ['CodeMirror-linenumbers', @get('foldGutterClass')]
      else
        cm.setOption 'lineNumbers', false
  ).observes 'showLineNumbers'

  rulerColumnDidChange: (->
    if cm = @get 'codeMirror'
      if @get('rulerColumn')
        cm.setOption 'rulers', [{ column: @get('rulerColumn') }]
      else
        cm.setOption 'rulers', null
  ).observes 'rulerColumn'

  showTrailingSpaceDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'showTrailingSpace', @get('showTrailingSpace')
  ).observes 'showTrailingSpace'

  matchBracketsDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'matchBrackets', @get('matchBrackets')
  ).observes 'matchBrackets'

  autoCloseBracketsDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'autoCloseBrackets', @get('autoCloseBrackets')
  ).observes 'autoCloseBrackets'

  keybindingsDidChange: (->
    if cm = @get 'codeMirror'
      cm.setOption 'keyMap', @get('keybindings')
  ).observes 'keybindings'

  textDidChange: (->
    if cm = @get 'codeMirror'
      if cm.getValue() != (text = @get 'text')
        cm.setValue text
  ).observes 'text'

  _bootstrapCodeMirror: ->
    cm = CodeMirror @get('domTarget'),
      lint              : @get 'lintableMode'
      keyMap            : @get 'keybindings'
      theme             : @get 'theme'
      mode              : @get 'mode'
      tabSize           : @get 'tabSize'
      indentUnit        : @get 'tabSize'
      indentWithTabs    : !@get 'softTabs'
      lineWrapping      : @get 'wrapLines'
      styleActiveLine   : @get 'highlightLine'
      lineNumbers       : @get 'showLineNumbers'
      showTrailingSpace : @get 'showTrailingSpace'
      matchBrackets     : @get 'matchBrackets'
      autoCloseBrackets : @get 'autoCloseBrackets'
      foldGutter        : { gutter: @get('foldGutterClass') }
      foldOptions       : { widget: '\u2026', scanUp: true, minFoldSize: 2 }
      gutters           : ['CodeMirror-linenumbers', @get('foldGutterClass')]
      value             : @get 'text'
      styleSelectedText : true # Selected text gets '.CodeMirror-selectedtext' class.
      showCursorWhenSelecting: true
      highlightSelectionMatches: true

    if rulerColumn = @get 'rulerColumn'
      cm.setOption 'rulers', [{ column: rulerColumn }]

    @_setupExtraKeys(cm)
    @_overrideKeys(cm)

    cm.on 'change', (cm, data) =>
      if @get('text') != (content = cm.getValue())
        @set 'text', content

    cm.on 'gutterClick', (cm, line, gutter, e) ->
      return if gutter != 'CodeMirror-linenumbers'

      others = []
      if e.ctrlKey || e.metaKey
        others = cm.listSelections()
      from = line
      to = line + 1

      update = ->
        others.push
          anchor: CodeMirror.Pos(from, 0)
          head: CodeMirror.Pos(to, 0)
        cm.setSelections(others, others.length - 1, origin: '*mouse')
      update()

      move = (e) ->
        curLine = cm.lineAtHeight(e.clientY, 'client')
        if (curLine != to)
          to = curLine
          update()

      up = ->
        window.removeEventListener('mouseup', up)
        window.removeEventListener('mousemove', move)

      window.addEventListener('mouseup', up)
      window.addEventListener('mousemove', move)

    @set 'codeMirror', cm

  _setupExtraKeys: (cm) ->
    if App.util.IsMac
      cm.addKeyMap(@get('macExtraKeys'))
    else
      cm.addKeyMap(@get('pcExtraKeys'))

  _overrideKeys: (cm) ->
    overrides = if App.util.IsMac then @get('macOverrideKeys') else @get('pcOverrideKeys')
    submap = if App.util.IsMac then 'macDefault' else 'pcDefault'
    _.each overrides, (key) ->
      delete(CodeMirror.keyMap[submap][key])

klass.reopenClass
  SETTINGS: [
    'softTabs'
    'tabSize'
    'theme'
    'wrapLines'
    'highlightLine'
    'showLineNumbers'
    'rulerColumn'
    'showTrailingSpace'
    'matchBrackets'
    'autoCloseBrackets'
    'keybindings'
  ]
