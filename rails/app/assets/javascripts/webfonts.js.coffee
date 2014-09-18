#= require_self
#= require webfontloader

window.WebFontConfig =
  google:
    families: [
      'Roboto:400,100,300,300italic,100italic,400italic,500,500italic,700,700italic:latin'
      'Source+Code+Pro:400,700:latin'
    ]

  active: ->
    window.setTimeout(->
      window.webFontLoaded?()
    , 1)
