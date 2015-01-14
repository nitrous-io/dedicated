#= require emberjs
#= require_self
#= require app-classes

window.Nitrous = Em.Application.extend Em.Evented,
  rootElement: '#app'

  platform:
    if navigator.platform.indexOf('Linux') != -1
      'linux'
    else if navigator.platform.indexOf('Windows') != -1
      'windows'
    else 'mac'

  online: if !Em.isNone(window.navigator.onLine) then window.navigator.onLine else true
  offline: Em.computed.not 'online'

  title: null
  snackbar: null

  settings: null

  token: null
  user: null
  userProfile: null
  hosts: null
  containers: null
  selectedHost: null
  selectedContainer: null
  selectedAppHost: null
  regions: null
  plans: null
  templates: null
  billingUsage: null

  terminalJsPath: null
  terminalCssPath: null

  eventsSocket: null

  pendingHost: null

  availableCredit: (->
    if @get('newUser')
      if @get('settings.new_user_credit')
        return @get('settings.new_user_credit')
      else
        return 5
    else if @get('billingUsage')
      return @get('billingUsage.availableCreditsDollars')

    return 0
  ).property 'newUser', 'billingUsage'

  availableCreditClass: (->
    if @get('availableCredit') <= 0
      return 'no-credit'
    else
      return 'good-credit'
  ).property 'availableCredit'

  newUser: (->
    @get('billingProfile') && @get('billingProfile.cardRequired')
  ).property 'billingProfile', 'billingProfile.cardRequired'

  desktopDownloadUrl: (->
    "http://getpro.nitrous.io/#{@get('platform')}/download.html"
  ).property 'platform'

  didChangeTitle: (->
    title = @get 'title'
    document.title = if title
      "#{title} · Nitrous"
    else
      "Nitrous"
  ).observes 'title'

  didChangeToken: (->
    if token = @get('token')
      window.localStorage.setItem('token', token)
    else
      window.localStorage.removeItem('token')
  ).observes 'token'

  didLoadSettings: (->
    if settings = @get('settings')
      if settings.mixpanel_token != '' && settings.mixpanel_token != 'disable'
        window.mixpanel?.init settings.mixpanel_token
      if settings.stripe_key != '' && settings.stripe_key != 'disable'
        window.Stripe?.setPublishableKey(settings.stripe_key)
  ).observes 'settings'

  noHost: (->
    @get('hosts.loaded') && @get('hosts.length') == 0
  ).property 'hosts.loaded', 'hosts.length'

  # TODO: Get this logic from server
  canCreateHosts: (->
    return true if @get('billingProfile.paid')
    if maxHosts = @get('userProfile.maxHosts')
      @get('hosts.loaded') && @get('hosts.length') < maxHosts
  ).property 'billingProfile.paid', 'userProfile.maxHosts', 'hosts.loaded', 'hosts.length'

window.App = Nitrous.create
  token: window.localStorage.getItem('token')

window.Lib = Ember.Namespace.create()

App.on 'load', ->
  App.__container__.lookup('adapter:application') # init app adapter class
  App.set 'snackbar', App.Snackbar.create()
  App.set 'regions', App.Region.fetch().value
  App.set 'plans', App.Plan.fetch().value
  App.set 'templates', App.Template.fetch().value

App.on 'fetchUser', (user) ->
  App.set 'user', user

  userProfile = App.UserProfile.create()
  userProfile.load user.get('_userProfile')
  App.set 'userProfile', userProfile

  billingProfile = App.BillingProfile.create()
  billingProfile.load user.get('_billingProfile')
  App.set 'billingProfile', billingProfile

  App.refreshUsage()

  App.set 'eventsSocket', socket = App.EventsSocket.create()
  socket.connect()

App.loadSettings = ->
  promise = Em.RSVP.Promise.cast(Em.$.ajax("/api/v0/app_settings"))
  promise.then (settings) ->
    App.set 'settings', settings
  promise

App.refreshUsage = ->
  App.BillingUsage.fetchCurrentUsage()

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

  beforeModel: (transition) ->
    @_super.apply this, arguments
    if !App.get('settings')
      transition.abort()
      App.loadSettings().then ->
        transition.retry()
      , (jqxhr) ->
        # retry in 5 seconds
        Em.run.later(this, ->
          transition.retry()
        , 5000)
      return

App.Router.reopen
  notifyGoogleAnalytics: (->
    ga? 'send', 'pageview', 'page': @get('url'), 'title': App.get('title')
  ).on 'didTransition'
