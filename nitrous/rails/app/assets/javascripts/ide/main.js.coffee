#= require ./vendor/underscore
#= require ./vendor/mousetrap
#= require ./codemirror
#= require ./vendor/base64
#= require ./vendor/strftime
#= require ./vendor/jquery.ui.widget
#= require ./vendor/jquery.fileupload
#= require ./ember-bootstrap
#= require ../lib/mp
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

  user: null
  userSettings: null
  container: null
  hostBinding: Em.Binding.oneWay 'container.host'

  terminalJsPath: null
  terminalCssPath: null

  consoleFullscreen: false
  consoleMinimized: false
  fileTreeHidden: false

  fiveMinWarned: false
  oneMinWarned: false
  redirecting: false

  didChangeToken: (->
    if token = @get('token')
      window.localStorage.setItem('token', token)
    else
      window.localStorage.removeItem('token')
  ).observes 'token'

  activateMixpanel: (->
    if @get('user') && settings = @get('settings')
      Mp.init(settings.mixpanel_token)
  ).observes 'user', 'settings'

  dashboardURL: ( ->
    "/app/#/containers/#{App.get 'container.slug'}"
  ).property 'App.container.slug'

  reloadContainer: ->
    @loadContainer(@get 'slug').then (container) ->
      container.loadHost()

  loadContainer: (slug)->
    if !slug
      return Em.RSVP.Promise.reject(new Error('No slug specified'))

    App.set 'slug', slug
    App.Container.fetch(slug).then (container) ->
      App.set 'container', container
      container

  loadSettings: ->
    promise = Em.RSVP.Promise.cast(Em.$.ajax("/api/v0/app_settings"))
    promise.then (settings) ->
      App.set 'settings', settings
    promise

  didChangeTitle: (->
    title = @get 'title'
    document.title = if title
      "#{title} · Nitrous"
    else
      "Nitrous"
  ).observes 'title'

  didChangeSlug: (->
    slug = App.get 'slug'
    App.set 'title', "#{slug} · IDE"
  ).observes 'slug'

  # thanks to http://www.rvdh.de/2014/11/14/time-based-triggers-in-ember-js/
  currentTimeMetronome: (->
    interval = 1000
    Ember.run.later this, =>
      this.notifyPropertyChange('currentTimePulse')
      this.currentTimeMetronome()
    , interval
  ).on 'init'

  usage: (->
    if !@get('host') || !@get('user.free')
      return

    primaryHost = @get('host')
    unless primaryHost && (date = new Date(primaryHost.last_running_at))
      return

    # if this is container shared by another user, don't apply usage restrictions
    # that way free user accessing container shared by paid user won't be kicked out
    user = @get('user')
    if primaryHost.user_id != user.id
      return

    now = (new Date()).getTime()
    refreshAt = new Date(date.getTime() + (@get('user.refreshPeriod')*1000))
    used = (now - date.getTime())/1000

    remaining = 0
    refreshIn = null
    if (r = refreshAt.getTime()) < now
      remaining = App.get('user.maxRunningTime')
    else
      remaining = App.get('user.maxRunningTime') - used
      refreshIn = (r - now)/1000

    remaining = 0 if remaining < 0

    {
      refreshIn: refreshIn,
      remaining: remaining,
      expired: remaining == 0
    }
  ).property 'currentTimePulse', 'host.last_running_at', 'user.maxRunningTime', 'user.refreshPeriod', 'user.free'

  usageExpired: (->
    return false unless @get 'usage'
  )

  notifyUsageWarning: (->
    return unless @get('host.state') == 'running'
    return unless @get 'usage'
    return if @get 'redirecting'
    usage = @get 'usage'

    if usage.remaining <= 0
      $(window).unbind "beforeunload.ide"
      @set 'redirecting', true
      window.location.href = @get 'dashboardURL'

    return if @get 'oneMinWarned'

    if usage.remaining <= 60
      App.snackbar.post 'Your workspace will be shutdown in less than a minute, please save your work.', sticky: true
      @set 'oneMinWarned', true
      return

    return if @get 'fiveMinWarned'

    if usage.remaining <= (5 * 60)
      App.snackbar.post 'Your workspace will be shutdown in less than 5 minutes, please save your work.', sticky: true
      @set 'fiveMinWarned', true
  ).observes 'currentTimePulse', 'usage'

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
    window.location.href = "/app/#/logout"
