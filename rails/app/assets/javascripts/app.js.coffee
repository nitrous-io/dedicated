#= require emberjs
#= require_self
#= require app-classes

window.Nitrous = Em.Application.extend Em.Evented,
  rootElement: '#app'

  online: if !Em.isNone(window.navigator.onLine) then window.navigator.onLine else true
  offline: Em.computed.not 'online'

  title: null
  snackbar: null

  token: null
  user: null
  hosts: null
  containers: null
  selectedHost: null
  selectedContainer: null
  regions: null
  plans: null
  baseImages: null

  terminalJsPath: null
  terminalCssPath: null

  eventsSocket: null

  didChangeTitle: (->
    title = @get 'title'
    document.title = if title
      "#{title} · Nitrous.IO"
    else
      "Nitrous.IO"
  ).observes 'title'

  didChangeToken: (->
    if token = @get('token')
      window.localStorage.setItem('token', token)
    else
      window.localStorage.removeItem('token')
  ).observes 'token'

  noHost: (->
    @get('hosts.loaded') && @get('hosts.length') == 0
  ).property 'hosts.loaded', 'hosts.length'

window.App = Nitrous.create
  token: window.localStorage.getItem('token')

window.Lib = Ember.Namespace.create()

App.on 'load', ->
  App.__container__.lookup('adapter:application') # init app adapter class
  App.set 'snackbar', App.Snackbar.create()

App.on 'fetchUser', (user) ->
  App.set 'user', user
  App.set 'eventsSocket', socket = App.EventsSocket.create()
  socket.connect()

App.didChangeTitle()

# prevent file drops on the document causing browser to navigate away
$(document).bind 'drop dragover', (e) ->
  e.preventDefault()

# prevent esc from disconnecting websocket connection in firefox
$(window).keydown (e) ->
  e.preventDefault() if e.keyCode == 27

$(window).on 'offline', ->
  window.App?.set 'online', false

$(window).on 'online', ->
  window.App?.set 'online', true

$(document).on 'click', '.noclick', (e) ->
  e.preventDefault()
  e.stopPropagation()

$(document).on 'click', '.click-highlight', (e) ->
  sel = window.getSelection()
  range = document.createRange()
  range.selectNodeContents(e.target)
  sel.removeAllRanges()
  sel.addRange(range)

Em.Route.reopen
  activate: ->
    App.set 'title', null
