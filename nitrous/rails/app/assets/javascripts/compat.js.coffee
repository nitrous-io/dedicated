#= require modernizr.custom

unsupported = false

`with(Modernizr) {
  unsupported = !(applicationcache && backgroundsize && borderradius && boxshadow &&
    cssanimations && cssgradients && csstransforms && csstransforms3d && csstransitions &&
    draganddrop && flexbox && fontface && generatedcontent && hashchange && history &&
    input.autocomplete && input.autofocus && input.min && input.max && input.multiple && input.pattern &&
    inputtypes.email && inputtypes.number &&
    localstorage && opacity && postmessage && rgba && svg && textshadow && websockets);
}`

return unless unsupported

style = document.createElement('style')
css = """
  #browser-warning {
    text-align: center;
    background: white;
    padding: 20px;
    font-family: sans-serif;
  }

  #browser-warning h3 {
    margin: 0 0 24px 0;
    font-size: 28px;
    color: black;
    font-weight: bold;
  }

  #browser-warning p {
    color: black;
    margin: 10px;
  }

  #browser-warning ul {
    margin: 30px 0 0 0;
    padding: 0;
    list-style: none;
  }

  #browser-warning li a {
    width: 320px;
    display: block;
    margin: 10px auto;
    height: 30px;
    line-height: 30px;
    color: white;
    background: #ccc;
    border-bottom: 2px solid #dedede;
  }

  #browser-warning li a.firefox {
    background: #da7319;
  }

  #browser-warning li a.chrome {
    background: #4285f4;
  }

  #browser-warning li a.opera {
    background: #ce0804;
  }
"""

style.type = 'text/css'
if style.styleSheet
  style.styleSheet.cssText = css
else
  style.appendChild(document.createTextNode(css))

head = document.head || document.getElementsByTagName('head')[0]
head.appendChild(style)

div = document.createElement('div')
div.id = 'browser-warning'
div.innerHTML = """
  <h3>Warning: Your browser is not supported!</h1>
  <p>Nitrous uses the latest standard in HTML5 technologies and requires an up-to-date web browser.</p>
  <p>Please upgrade to one of the following supported web browsers and try again:</p>
  <ul>
    <li><a class="chrome" href="https://www.google.com/chrome/browser/desktop/" target="_blank">Google Chrome (Recommended)</a></li>
    <li><a class="firefox" href="https://www.firefox.com/download" target="_blank">Firefox</a></li>
    <li><a class="opera" href="http://www.opera.com/download" target="_blank">Opera</a></li>
  </ul>
"""

document.body.insertBefore(div, document.body.firstChild)
