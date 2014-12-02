#= require ./tab-content

App.SearchResults = App.TabContent.extend
  isSearchTab: true

  content: ''
  path: null
  contentType: 'search-results'

  term: null
  results: null
  errors: null
  currentResult: null
  fileCount: 0
  matchCount: 0
  completed: false

  init: ->
    @_super()
    @set 'results', Ember.A([])
    @set 'errors', Ember.A([])
    @set 'currentResult', this.emptyResult()

  doComplete: ->
    @set 'completed', true
    App.util.mixpanel 'Viewed Find in Project Result',
      matches: @get('matchCount')
      files: @get('fileCount')
      success: (@get('errors') || []).length == 0

  title: (->
    if term = @get('term')
      termTitle = if term.length > 6 then term.slice(0, 6) + "..." else term
      "Search results: '#{termTitle}'"
    else
      "Searching..."
  ).property 'term'

  tooltip: (->
    "Search results: '#{@get('term')}'"
  ).property 'term'

  status: (->
    termText = "'#{@get('term')}'"
    locationSuffix = if @get('path') == '.' then '' else "in #{@get('path')}"
    if @get('matchCount') == 0 && @get('fileCount') == 0
      if @get('completed') then "No results found searching for #{termText} #{locationSuffix}" else 'Searching...'
    else
      [
        App.util.pluralize @get('matchCount'), 'match', 'matches'
        'in'
        App.util.pluralize @get('fileCount'), 'file', 'files'
        "searching for #{termText}"
        locationSuffix
      ].join(' ')
  ).property 'fileCount', 'matchCount', 'completed', 'term'

  addSearchResults: (data) ->
    lines = data.split("\n")
    _.each lines, (rawLine) =>
      currentResult = @get 'currentResult'
      switch
        # ag --ackmate format
        # :filename.ext           Filename of match
        when rawLine[0] == ':'
          this.addCurrentResult()
          @set 'currentResult.filename', rawLine.slice(1)
        when rawLine.trim() == ''
          # no-op
        else
          [line, matchCount] = this.parseResultLine(rawLine)
          currentResult.incrementProperty('matchCount', matchCount)
          currentResult.get('lines').pushObject(line)

  emptyResult: ->
    Em.Object.create
      filename: ''
      lines: []
      matchCount: 0

  addCurrentResult: ->
    currentResult = @get 'currentResult'
    return if currentResult.get('filename') == ''
    @get('results').pushObject(currentResult)
    @incrementProperty 'fileCount'
    @incrementProperty 'matchCount', currentResult.get('matchCount')
    @set 'currentResult', this.emptyResult()

  parseResultLine: (rawLine) ->
    # ag --ackmate format
    # 123:linetext            no matches on line 123, just context
    # 123;45 6:linetext       single match on line 123 byte 45, match is 6 bytes long
    # 123;45 6,78 9:linetext  multiple matches on line 123: byte 45 6 bytes long, byte 78 9 bytes long
    # NOTE: column and length numbers are *byte* count
    [metadata, lineTextParts...] = rawLine.split(':')
    [lineNumber, matchInfo] = metadata.split(';')

    matches = if matchInfo then matchInfo.split(',') else []
    lineText = lineTextParts.join(':')

    if lineText == ''
      return [{ lineNumber: lineNumber, lineParts: []}, 0]
    if matches.length == 0
      # Context lines have no matches by definition.  Presently there's no
      # option to turn these on, so this is just here for
      # correctness/completeness.
      # The algorithm below works for matches == []; returning early to avoid
      # unnecessary byte conversions
      return [
        lineNumber: lineNumber, lineParts: [{text: lineText, match: false}]
        0
      ]

    lineUtf8 = unescape(encodeURIComponent(lineText))
    lineBytes = (lineUtf8.charCodeAt(i) for i in [0..lineUtf8.length-1])

    lineParts = []

    pushBytes = (bytes, isMatch) ->
      if bytes.length > 0
        lineParts.push
          text: decodeURIComponent(escape(String.fromCharCode.apply(null, bytes)))
          match: isMatch

    currentIndex = 0
    _.each matches, (matchData) ->
      # each iteration through the loop adds at most 2 lineParts -- everything
      # up to the match, and the match itself
      [startIndex, matchLength] = _.map matchData.split(' '), (s) -> parseInt(s)
      partBytes = lineBytes.slice(currentIndex, startIndex)
      pushBytes(partBytes, false)
      partBytes = lineBytes.slice(startIndex, startIndex + matchLength)
      pushBytes(partBytes, true)
      currentIndex = startIndex + matchLength
    # last unmatched fragment
    partBytes = lineBytes.slice(currentIndex)
    pushBytes(partBytes, false)

    return [{ lineParts: lineParts, lineNumber: lineNumber}, matches.length]

  addError: (errorSlug) ->
    if errorMessage = this.friendlyError(errorSlug)
      @get('errors').pushObject errorMessage

  friendlyError: (errorSlug) ->
    slug = errorSlug.trim()
    switch
      when slug == 'LIMIT'
        "Search returned too many results.  Some results have been omitted.  Please try a more specific search"
      when slug == 'NOPATH'
        "Could not find path #{@get('path')}.  Please check if it exists"
      when slug == 'BADREGEX'
        "Error in given regular expression.  Please double-check your search term and make sure parentheses, brackets, etc all match."
      when slug.match /MAXMATCH/
        [..., filename] = slug.split(':')
        "Too many matches in '#{filename}'.  Skipping the rest of the file."
      else
        null
