Em.Handlebars.helper 'ago', (date) ->
  if date instanceof Date
    $.timeago(date)

Em.Handlebars.helper 'utc', (date) ->
  if date instanceof Date
    date.toUTCString()

Em.Handlebars.helper 'simpledate', (date) ->
  if date instanceof Date
    date.toDateString()

Em.Handlebars.helper 'dateabbr', (date) ->
  if date instanceof Date
    new Ember.Handlebars.SafeString(Em.$('<abbr>').attr('title', date.toString()).text($.timeago(date))[0].outerHTML)
