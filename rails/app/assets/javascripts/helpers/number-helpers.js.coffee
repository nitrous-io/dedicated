Em.Handlebars.helper 'mebibytes', (num) ->
  if typeof num == 'number'
    (num / (1024*1024)).toFixed(2)

Em.Handlebars.helper 'gibibytes', (num) ->
  if typeof num == 'number'
    (num / (1024*1024*1024)).toFixed(2)

Em.Handlebars.helper 'toFixed', (num, digits) ->
  if typeof num == 'number'
    num.toFixed(digits)
