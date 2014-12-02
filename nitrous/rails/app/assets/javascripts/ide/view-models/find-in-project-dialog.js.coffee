App.FindInProjectDialog = Em.Object.extend
  errors: null
  term: null
  location: null

  optionCaseSensitive: false
  optionRegex: false

  valid: ->
    errors = {}
    if (@get('term') || '').trim() == ''
      errors.term = 'is required'

    @set 'errors', errors

    return _.isEmpty(errors)

  optionsString: (->
    options = []
    options.push('ignoreCase') if !@get('optionCaseSensitive')
    options.push('literal') if !@get('optionRegex')
    options.join(',')
  ).property 'optionCaseSensitive', 'optionRegex'
