#= require ./tab-content

App.SearchResults = App.TabContent.extend
  isSearchTab: true

  content: ''
  path: null
  contentType: 'search-results'

  term: null
  results: null
  errors: null
  fileCount: 0
  matchCount: 0
  completed: false

  init: ->
    @_super()
    @set 'results', Ember.A([])
    @set 'errors', Ember.A([])

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
    if @get('matchCount') == 0 && @get('fileCount') == 0
      if @get('completed') then "No results found searching for #{termText} in #{@get('path')}" else 'Searching...'
    else
      [
        App.util.pluralize @get('matchCount'), 'match', 'matches'
        'in'
        App.util.pluralize @get('fileCount'), 'file', 'files'
        "searching for #{termText} in #{@get('path')}"
      ].join(' ')
  ).property 'fileCount', 'matchCount', 'completed', 'term'

  completedStatus: (->
    return unless @get('completed')
    [
        'Search completed:'
        App.util.pluralize @get('matchCount'), 'match', 'matches'
        'in'
        App.util.pluralize @get('fileCount'), 'file', 'files'
    ].join(' ')
  ).property 'fileCount', 'matchCount', 'completed'

  addSearchResults: (newResults) ->
    _.each newResults, (res) =>
      # file: "test.rb", matches: [content: "import foofoo", row: 1, count: 2]
      return if !res.matches || res.matches.length == 0

      result = Em.Object.create
        filename: [@get('path'), res.file].join('/')
        matchCount: res.matches.length
        lines: []

      _.each res.matches, (match) =>
        line = Em.Object.create
          lineNumber: match.row
          text: match.content
        result.get('lines').pushObject(line)
        @incrementProperty 'matchCount', match.count

      @get('results').pushObject(result)
      @incrementProperty 'fileCount'

  addError: (error) ->
    @get('errors').pushObject(error)
