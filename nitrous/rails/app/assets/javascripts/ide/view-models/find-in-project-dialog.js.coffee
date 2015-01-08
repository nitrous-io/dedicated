App.FindInProjectDialog = Em.Object.extend
  term: null
  location: null

  optionCaseSensitive: false
  optionRegex: false

  optionsString: (->
    options = []
    options.push('ignoreCase') if !@get('optionCaseSensitive')
    options.push('literal') if !@get('optionRegex')
    options.join(',')
  ).property 'optionCaseSensitive', 'optionRegex'
