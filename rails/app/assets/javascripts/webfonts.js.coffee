window.WebFontConfig =
  google:
    families: [
      'Roboto:400,100,300,300italic,100italic,400italic,500,500italic,700,700italic:latin'
      'Source+Code+Pro:400,700:latin'
    ]

wf = document.createElement('script')
protocol = if 'https:' == document.location.protocol
  'https'
else
  'http'
wf.src = "#{protocol}://ajax.googleapis.com/ajax/libs/webfont/1/webfont.js"
wf.type = 'text/javascript'
wf.async = 'true'
wf.onload = ->
  window.webFontLoaded?()
s = document.getElementsByTagName('script')[0]
s.parentNode.insertBefore(wf, s)
