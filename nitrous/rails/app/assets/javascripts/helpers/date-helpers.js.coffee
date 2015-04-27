Em.Handlebars.helper 'ago', (date) ->
  if date instanceof Date
    $.timeago(date)

Em.Handlebars.helper 'utc', (date) ->
  if date instanceof Date
    date.toUTCString()

Em.Handlebars.helper 'simpledate', (date) ->
  if date instanceof Date
    date.toDateString()

Em.Handlebars.helper 'basicdatetime', (date) ->
  unless date instanceof Date
    return
  yr = date.getFullYear()
  mnt = (date.getMonth() + 1)
  day = (date.getDate())
  hr = date.getHours()
  if hr == 0
    hr = 12
  min = date.getMinutes()

  "#{yr}/#{util.padZeroes(mnt)}/#{util.padZeroes(day)} #{util.padZeroes(hr)}:#{util.padZeroes(min)}"

Em.Handlebars.helper 'dateabbr', (date) ->
  if date instanceof Date
    new Ember.Handlebars.SafeString(Em.$('<abbr>').attr('title', date.toString()).text($.timeago(date))[0].outerHTML)
