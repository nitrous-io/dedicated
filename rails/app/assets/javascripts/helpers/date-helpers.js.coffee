Em.Handlebars.helper 'ago', (date) ->
  if date instanceof Date
    $.timeago(date)

Em.Handlebars.helper 'utc', (date) ->
  if date instanceof Date
    date.toUTCString()
