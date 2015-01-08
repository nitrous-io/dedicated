window.util =
  IsSafari:
    navigator.userAgent.indexOf("Safari") != -1 && navigator.userAgent.indexOf("Chrome") == -1

  escapeRegex: (text) ->
    text.replace(/([.*+?^${}()|\[\]\/\\])/g, "\\$1")

  randomInteger: (min, max) ->
    Math.floor(Math.random() * ((max + 1) - min)) + min
