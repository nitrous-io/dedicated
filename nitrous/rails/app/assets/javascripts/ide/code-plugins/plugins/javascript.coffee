#= require ./base
#= require ./global-javascript

klass = class Javascript extends PluginBase

  HINT_WORDS = [
    'document'
    'else'
    'if'
    'parseInt'
    'return'
    'setTimeout'
    'window'
  ]

  triggerCtagsUpdate: ->
    return unless @document?.isOTMaster()
    @globalPlugin.triggerCtagsUpdate(@getRootPath().get('absolutePath'))

  afterInitialize: ->
    super
    @triggerCtagsUpdate()

  afterSetCodeMirror: ->
    @processFile()

  afterSave: (context)->
    @triggerCtagsUpdate()
    @processFile()

  afterChanges: (changes)->
    for change in changes
      if change.origin?.match(/delete/)
        @processFile()
        break

      # process the existing text before asking CM
      process = false
      if lines = change.text
        for line in lines
          if line.match(/require/)
            process = true
            break

      if process
        @processFile()
        break

      # okay finally ask for any changes
      start = {line: change.from.line, ch: 0}
      ending = change.to
      text = @codeMirror.getRange(start, ending)
      if text.match(/require/)
        @processFile()
        break

  handleFileChanged: (change, location)->
    if location && location.match(/\.js$/)
      @triggerCtagsUpdate()

  handleDirChanged: ->
    @triggerCtagsUpdate()

  processFile: ->
    @definitionEndsAt = 0
    @requirements = []
    @eachLine (line) =>

      requires = line.text.match(/require\(?["']([^'"]+)["']\)?/)
      if requires
        @requirements.push requires[1]

      return false

  cTagsLocation: ->
    @getRootPath().get('absolutePath') + "/.js-tags"

  ctagsUpdated: ->
    if exec = @afterCtagsUpdated
      @afterCtagsUpdated = null
      exec()

  autocomplete: (cm, callback, options)->
    options ||= {}
    options.handlers = {
      pick: [
        ->
          Mp.track("IDE Autocomplete Picked JS")
        ]
    }

    cursor = cm.getCursor()
    line = cm.getRange({line: cursor.line, ch: 0}, cursor)
    unless line && line.length
      callback(list: [])
      return

    @autocompleteLine(line, cursor, cm, callback, options)

  autocompleteLine: (line, cursor, cm, callback, options)->
    renderFunc = (el, cm, data)=>
      html = "<span class=\"text\">#{data.text}</span>"
      if data.file
        html = "#{html}<span class=\"file\">#{data.file}</span>"
      el.innerHTML = html

    # look for symbols after a space, period or colon
    # if we are doing a class definition with a base class
    # autocomplete on the base class
    symbols = line.match(/\s?([^\[\]{}\|(); .:'"\/?!%\^*\-]+)$/)
    unless symbols
      callback(_handlers: options.handlers, list: [])
      return

    newClass = null
    symbol = symbols[1]

    # store featured results
    localHints = CodeMirror.hint.auto(@codeMirror, {})
    suggestedResults = {}
    if list = localHints?.list
      for hint in list
        if hint != symbol
          suggestedResults[hint] = {text: hint, file: null, render: renderFunc}

    @scanListForSymbol HINT_WORDS, suggestedResults, symbol, renderFunc

    # let local process run
    callback(_handlers: options.handlers, list: @hashToResults(suggestedResults), to: cursor, from: {line: cursor.line, ch: (cursor.ch - symbol.length)})

    results = {} # all other results
    if @afterCtagsUpdated
      return

    if symbol.length >= 2
      @searchCTags "^#{symbol}", (err, cmdErr, stdout, stderr) =>
        if stderr && stderr.match(/\.js-tags:\s+No\s+such/)
          @afterCtagsUpdated = =>
            @autocompleteLine(line, cursor, cm, callback, options)
          @triggerCtagsUpdate()
          return

        @parseCTags stdout, (varName, path, allParts)=>
          # no point suggesting the same thing
          if symbol == varName
            return

          if path == @absolutePath
            suggestedResults[varName] =  {text: varName, file: "this", render: renderFunc}
            delete results[varName]

          # check if the symbol is in a node_module and filter
          if path && (pkgName = path.match(/node_modules\/([^/]+)/)) && @requirements
            # ensure the file is required
            found = false
            for requirement in @requirements
              if requirement.indexOf(pkgName[1]) != -1
                found = true
                break

            unless found
              return

          unless results[varName] || suggestedResults[varName]
            file = null
            if path && (split = path.split("/"))
              file = split[split.length-1]
              if split.length >= 3 # if the folder is heavily nested
                folder = split[split.length-2]
                file = "#{folder}/#{file}"
            results[varName] = {text: varName, file: file, render: renderFunc}

        callback(_handlers: options.handlers, list: @hashToResults(suggestedResults, results), to: cursor, from: {line: cursor.line, ch: (cursor.ch - symbol.length)})

klass.beta = true
App.CodePluginsManager.registerCodePlugin "javascript", klass, ["javascript", "text/x-javascript", "text/javascript"]