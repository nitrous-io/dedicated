#= require_self
#= require webfontloader

window.WebFontConfig =
  google:
    families: [
      # 'Roboto:400,100,300,300italic,100italic,400italic,500,500italic,700,700italic:latin'
      # 'Roboto:100,300,400,500,700:latin'
      # 'Source+Code+Pro:400,700:latin'
      # 'Source+Sans+Pro:200,700'
      'Open+Sans:400,300,300italic,600,700:latin'
      'Montserrat:700,400'
    ]

  active: ->
    window.setTimeout(->
      window.webFontLoaded?()
    , 1)
