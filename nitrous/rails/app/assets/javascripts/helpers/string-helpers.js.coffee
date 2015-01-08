Em.Handlebars.helper 'truncate', (text, limit) ->
  _limit = limit || 50
  if text.length > limit
    return text.substr(0, limit) + '…'
  text
