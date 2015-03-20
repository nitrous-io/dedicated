#= require_self
#= require webfontloader

window.WebFontConfig =
  google:
    families: [
      'Roboto:100,300,300italic,400,500,700:latin'
      'Open+Sans:400,300,300italic,600,700:latin'
      'Montserrat:700,400'
    ]

  active: ->
    window.setTimeout(->
      window.webFontLoaded?()
    , 1)
