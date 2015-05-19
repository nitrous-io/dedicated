window.util =
  escapeRegex: (text) ->
    text.replace(/([.*+?^${}()|\[\]\/\\])/g, "\\$1")

  randomInteger: (min, max) ->
    Math.floor(Math.random() * ((max + 1) - min)) + min

  padZeroes: (num, length = 2)->
    str = "#{num}"
    if str.length == length
      return str
    required = length - str.length
    pad = Math.pow(10, required).toString().substr(1)
    "#{pad}#{num}"
