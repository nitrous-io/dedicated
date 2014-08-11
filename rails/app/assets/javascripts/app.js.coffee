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
  selectedHost: null
  selectedContainer: null

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

  clear: ->
    App.setProperties
      token: null
      user: null
      hosts: null
    Ember.notifyPropertyChange App.User, '_cache'
    Ember.notifyPropertyChange App.Host, '_cache'
    Ember.notifyPropertyChange App.Container, '_cache'

window.App = Nitrous.create
  token: window.localStorage.getItem('token')

App.on 'AppDidLoad', ->
  App.__container__.lookup('adapter:application') # init app adapter class
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
