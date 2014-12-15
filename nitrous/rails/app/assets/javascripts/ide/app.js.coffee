#= require ./vendor/mousetrap
#= require ./codemirror
#= require ./vendor/base64
#= require ./vendor/strftime
#= require ./vendor/jquery.ui.widget
#= require ./vendor/jquery.fileupload
#= require ./vendor/ua-parser
#= require ./ember-bootstrap
#= require_self
#= require ./app-classes

window.NitrousIO = Em.Application.extend Em.Evented,
  rootElement: '#app'

  online: if !Em.isNone(window.navigator.onLine) then window.navigator.onLine else true
  offline: Em.computed.not 'online'

  flashMessage: null

  slug: null
  token: null

  currentUser: null
  container: null

  terminalJsPath: null
  terminalCssPath: null

  didChangeToken: (->
    if token = @get('token')
      window.localStorage.setItem('token', token)
    else
      window.localStorage.removeItem('token')
  ).observes 'token'

  bootstrap: (opts={}) ->
    @set 'LOG_RESOLVER', true
    @set 'LOG_TRANSITIONS', true
    @set 'LOG_TRANSITIONS_INTERNAL', true
    @set 'LOG_VIEW_LOOKUPS', true

    @set 'terminalJsPath', opts.terminalJsPath
    @set 'terminalCssPath', opts.terminalCssPath

window.App = NitrousIO.create
  token: window.localStorage.getItem('token')
  slug: window.location.search.slice('1')

Em.TextSupport.reopen
  attributeBindings: ['readonly']

# HACK: silence bindAttr deprecation warning (bindAttrs are rendered by Hamlbars).
# Goes away when we can upgrade Hamlbars to 2.1+ or switch to HTMLbars/Emblem.
Em.Handlebars.registerHelper('bindAttr', Ember.Handlebars.helpers['bind-attr'])

# prevent file drops on the document causing browser to navigate away
$(document).bind 'drop dragover', (e) ->
  e.preventDefault()

# prevent esc from disconnecting websocket connection in firefox
$(window).keydown (e) ->
  e.preventDefault() if e.keyCode == 27

$(window).on 'offline', ->
  window.App?.set? 'online', false

$(window).on 'online', ->
  window.App?.set? 'online', true
