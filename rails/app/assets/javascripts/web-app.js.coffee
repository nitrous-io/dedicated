#= require emberjs
#= require_self
#= require app-classes

window.Nitrous = Em.Application.extend Em.Evented,
  rootElement: '#app'

  online: if !Em.isNone(window.navigator.onLine) then window.navigator.onLine else true
  offline: Em.computed.not 'online'

  title: null
  currentUser: null

  didChangeTitle: (->
    title = @get 'title'
    document.title = if title
      "#{title} · Nitrous.IO"
    else
      "Nitrous.IO"
  ).observes 'title'

window.App = Nitrous.create
  snackbar: null

App.on 'AppDidLoad', ->
  App.set 'snackbar', App.Snackbar.create()

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

Em.Route.reopen
  activate: ->
    App.set 'title', null
