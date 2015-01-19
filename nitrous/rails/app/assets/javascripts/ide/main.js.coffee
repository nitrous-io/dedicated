#= require ./vendor/underscore
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

  platform:
    if navigator.platform.indexOf('Linux') != -1
      'linux'
    else if navigator.platform.indexOf('Windows') != -1
      'windows'
    else 'mac'

  online: if !Em.isNone(window.navigator.onLine) then window.navigator.onLine else true
  offline: Em.computed.not 'online'

  settings: null

  config: null

  flashMessage: null

  slug: null
  token: null

  currentUser: null
  container: null

  terminalJsPath: null
  terminalCssPath: null

  editorFullscreen: false
  consoleFullscreen: false

  didChangeToken: (->
    if token = @get('token')
      window.localStorage.setItem('token', token)
    else
      window.localStorage.removeItem('token')
  ).observes 'token'

  activateMixpanel: (->
    if @get('currentUser') && settings = @get('settings')
      if settings.mixpanel_token != '' && settings.mixpanel_token != 'disable'
        window.mixpanel?.init settings.mixpanel_token
        App.util.mixpanelIdentifyUser()
  ).observes 'currentUser', 'settings'

  reloadContainer: ->
    new Em.RSVP.Promise (resolve, reject) =>
      @loadContainer ()->
        App.get('container')?.loadHost().then (v)->
          resolve(v)
        , (err) ->
          reject(err)

  loadContainer: (cb)->
    App.Container.fetch(@get('slug')).then (container) ->
      App.set 'container', container
      cb?()
      container
    , -> # failure
      window.location.href = notFoundPath

  loadSettings: ->
    promise = Em.RSVP.Promise.cast(Em.$.ajax("/api/v0/app_settings"))
    promise.then (settings) ->
      App.set 'settings', settings
    promise

window.App = NitrousIO.create
  token: window.localStorage.getItem('token')

window.Lib = Ember.Namespace.create()

App.on 'load', ->
  App.set 'snackbar', App.Snackbar.create()

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

$(window).on 'storage', (e) ->
  origEvent = e.originalEvent
  if origEvent.key == "token" && origEvent.newValue == null
    window.location.href = "/app#logout"
