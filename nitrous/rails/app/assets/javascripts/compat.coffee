#= require modernizr.custom

supported = false
localStorageSupported = false

`with(Modernizr) {
  supported = applicationcache && flexbox && fontface &&
    generatedcontent && hashchange && history && opacity && rgba && svg &&
    websockets;
  localStorageSupported = Modernizr.localstorage
}`

return if supported && localStorageSupported

style = document.createElement('style')
css = """
#browser-warning {
  text-align: center;
  background: #e6f2fb;
  padding: 20px;
  font-family: Helvetica, Arial, "DejaVu Sans", sans-serif;
  box-sizing: content-box;
}
#browser-warning * {
  box-sizing: content-box;
}
#browser-warning h3 {
  margin: 0 0 20px 0;
  font-size: 22px;
  color: black;
  font-family: inherit;
  font-weight: bold;
}
#browser-warning p {
  color: black;
  margin: 10px;
  font-family: inherit;
  font-weight: normal;
  font-size: 16px;
}
#browser-warning ul {
  margin: 20px 0 0 0;
  padding: 0;
  list-style: none;
}
#browser-warning li {
  display: inline-block;
}
#browser-warning .btn {
  display: inline-block;
  padding: 3px 10px;
  height: 30px;
  line-height: 30px;
  color: white;
  background: #039be5;
  border-radius: 4px;
  font-family: inherit;
  font-weight: normal;
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
#browser-warning p.no-local-storage {
  margin: 0;
}
#browser-warning .dismiss {
  margin-left: 8px;
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

window.dismissBrowserWarning = ->
  el = document.getElementById('browser-warning')
  el.parentNode.removeChild(el)

if supported && !localStorageSupported
  div.innerHTML = """
    <p class="no-local-storage">Are you in Private Browsing / Incognito mode? Certain HTML5 features appear to be disabled, and Nitrous may not function correctly. <a class='dismiss btn' href='#' onclick='dismissBrowserWarning()'>Dismiss</a></p>
  """
  # add basic html5 localstorage shim
  storage = {}
  window.localStorage ||= {}
  window.localStorage.removeItem = (key) ->
    delete storage[key]
  window.localStorage.setItem = (key, value) ->
    storage[key] = value
  window.localStorage.getItem = (key) ->
    storage[key]
  window.localStorage.clear = ->
    storage = {}
else
  div.innerHTML = """
    <h3>Warning: Your browser is not supported!</h1>
    <p>Nitrous uses the latest standard in HTML5 technologies and requires an up-to-date web browser.</p>
    <p>Please upgrade to one of the following supported web browsers and try again:</p>
    <ul>
      <li><a class="btn chrome" href="https://www.google.com/chrome/browser/desktop/" target="_blank">Google Chrome (Recommended)</a></li>
      <li><a class="btn firefox" href="https://www.firefox.com/download" target="_blank">Mozilla Firefox</a></li>
      <li><a class="btn opera" href="http://www.opera.com/download" target="_blank">Opera</a></li>
    </ul>
  """

document.body.insertBefore(div, document.body.firstChild)
