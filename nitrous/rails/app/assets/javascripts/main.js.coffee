#= require emberjs
#= require ./lib/mp
#= require_self
#= require app-classes

window.Nitrous = Em.Application.extend Em.Evented,
  rootElement: '#app'

  # supports variations of the Windows navigator.platform
  # see http://stackoverflow.com/questions/19877924/what-is-the-list-of-possible-values-for-navigator-platform-as-of-today
  platform:
    if navigator.platform.indexOf('Linux') != -1
      'linux'
    else if navigator.platform.indexOf('Win32') != -1 || navigator.platform.indexOf('Win64') != -1 || navigator.platform.indexOf('Windows') != -1
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
  sharedContainers: null
  selectedHost: null
  selectedContainer: null
  regions: null
  templates: null
  billingUsage: null

  eligiblePlans: null

  terminalJsPath: null
  terminalCssPath: null

  eventsSocket: null

  hostSaving: false
  pendingHost: null

  containerSaving: false
  pendingContainerData: null

  onboardingScreen: null
  onboardingReady: false
  onboardingIDEOpen: false
  temporaryScreen: null

  fiveMinWarned: false
  oneMinWarned: false
  zeroWarned: false

  regionIndex: (->
    regions = @get('regions')

    index = {}
    for region in regions.get('content')
      index[region.get('id')] = region

    return index
  ).property 'regions'

  planIndex: (->
    plans = @get('plans')

    index = {}
    for plan in plans.get('content')
      index[plan.get('id')] = plan

    return index
  ).property 'plans'

  groupedSharedContainers: (->
    set = []
    @get('sharedContainers').forEach (item)=>
      existing = set.findBy('userId', item.get('userId'))
      if existing
        existing.get('items').unshift item
      else
        existing = App.SharedContainersGroup.create
          userId: item.get('userId')
          items: [item]

        set.push existing
    set.sortBy('username')
  ).property 'sharedContainers.@each'

  showOnboarding: (->
    @get('user.firstTime') && @get('onboardingReady') && !@get('freeQueue')
  ).property 'onboardingReady', 'user.firstTime'

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
      App.set('adminToken', null)
      window.localStorage.removeItem('token')
  ).observes 'token'

  didChangeAdminToken: (->
    if token = @get('adminToken')
      window.localStorage.setItem('adminToken', token)
    else
      window.localStorage.removeItem('adminToken')
  ).observes 'adminToken'

  activateStripe: (->
    if settings = @get('settings')
      if settings.stripe_key != '' && settings.stripe_key != 'disable'
        window.Stripe?.setPublishableKey(settings.stripe_key)
  ).observes 'settings'

  activateMixpanel: (->
    if settings = @get('settings')
      Mp.init(settings.mixpanel_token)
  ).observes 'settings'

  identifyUserInMixpanel: (->
    if @get('user')
      Mp.identify()
  ).observes 'user', 'adminToken'

  noHost: (->
    @get('hosts.loaded') && @get('hosts.length') == 0
  ).property 'hosts.loaded', 'hosts.length'

  singleHost: (->
    @get('hosts.loaded') && @get('hosts.length') == 1
  ).property 'hosts.loaded', 'hosts.length'

  # thanks to http://www.rvdh.de/2014/11/14/time-based-triggers-in-ember-js/
  currentTimeMetronome: (->
    interval = 1000
    Ember.run.later this, =>
      this.notifyPropertyChange('currentTimePulse')
      this.currentTimeMetronome()
    , interval
  ).on 'init'

  usage: (->
    if @get('noHost') || !@get('billingProfile.free')
      return

    primaryHost = @get('hosts.firstObject')
    unless primaryHost && (date = primaryHost.get('lastRunningAt'))
      return

    now = (new Date()).getTime()
    maxRun = @get('user.maxRunningTime')
    refreshPeriod = @get('user.refreshPeriod')

    refreshAt = new Date(date.getTime() + (refreshPeriod*1000))
    used = (now - primaryHost.get('lastRunningAt').getTime())/1000

    remaining = 0
    refreshIn = null
    if (r = refreshAt.getTime()) < now
      remaining = maxRun
    else
      remaining = maxRun - used
      refreshIn = (r - now)/1000

    # correct for overruns or underuns
    if remaining > maxRun
      remaining = maxRun
    else if remaining < 0
      remaining = 0

    expired = remaining == 0
    percent = 0
    if expired
      percent = 100 - (((refreshPeriod - refreshIn) / refreshPeriod) * 100)
    else
      percent = ((remaining / maxRun) * 100)

    return {
      refreshIn: refreshIn,
      remaining: remaining,
      expired: expired,
      progressBarStyle: "width: #{percent}%"
    }
  ).property 'currentTimePulse', 'noHost', 'hosts.firstObject.lastRunningAt', 'user.maxRunningTime', 'user.refreshPeriod', 'billingProfile.free'

  notifyUsageWarning: (->
    return unless @get('hosts.firstObject.state') == 'running'
    return unless @get 'usage'

    return if @get 'zeroWarned'
    usage = @get 'usage'
    if usage.remaining <= 0
      App.snackbar.post 'You have exceeded your daily usage for the free tier, please consider upgrading.', sticky: true
      @set 'zeroWarned', true
      return

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

  flushWarnings: (->
    return unless @get('zeroWarned') && @get('oneMinWarned') && @get('fiveMinWarned')
    @set 'zeroWarned', false
    @set 'oneMinWarned', false
    @set 'fiveMinWarned', false
  ).observes 'hosts.firstObject.lastRunningAt'

  reloadEligiblePlans: (->
    @get('eligiblePlans')?.reload()
  ).observes 'billingProfile.state'

  restrictedAccount: Em.computed.equal 'billingProfile.state', 'delinquent'

  # TODO: Get this logic from server
  canCreateHosts: (->
    if @get('billingProfile.isNoCardState')
      return true

    if @get('billingProfile.free') && (@get('hosts.length') > 0)
      return false

    if !@get('billingProfile.active')
      return false

    if maxHosts = @get('userProfile.maxHosts')
      return @get('hosts.loaded') && (@get('hosts.length') < maxHosts)

    return true

  ).property('billingProfile.isNoCardState', 'billingProfile.free', 'billingProfile.active',
    'userProfile.maxHosts', 'hosts.loaded', 'hosts.length')

  logout: (newToken=null) ->
    Em.$.ajax(
      type: 'DELETE'
      url: "/api/v0/logout"
      cache: false
      headers: { Authorization: "Bearer #{@get 'token'}" }
    ).always ->
      App.set 'token', newToken
      Mp.track Mp.EventUserLoggedOut

  switchBackAdmin: ->
    adminToken = App.get('adminToken')
    App.set('adminToken', null)

    @logout(adminToken).then ->
      Em.run.next ->
        location.href = "/"

window.App = Nitrous.create
  token: window.localStorage.getItem('token')
  adminToken: window.localStorage.getItem('adminToken')

window.Lib = Ember.Namespace.create()

App.on 'load', ->
  App.__container__.lookup('adapter:application') # init app adapter class
  App.set 'snackbar', App.Snackbar.create()
  App.set 'plans', App.Plan.fetch().value
  App.set 'regions', App.Region.fetch().value
  App.set 'templates', App.Template.fetch().value
  Em.run.later(->
    Em.run.scheduleOnce 'afterRender', ->
      webapp?.loading.stop()
  , 100)

App.on 'fetchUser', (user) ->
  App.set 'user', user

  userProfile = App.UserProfile.create()
  userProfile.load user.get('_userProfile')
  App.set 'userProfile', userProfile

  billingProfile = App.BillingProfile.create()
  billingProfile.load user.get('_billingProfile')
  App.set 'billingProfile', billingProfile

  App.loadPlans()
  App.refreshUsage()
  App.refreshBillingProfile()
  App.refreshInvoices()
  App.loadSettings()

  # make sure that we only set eventsSocket once
  unless App.get 'eventsSocket'
    App.set 'eventsSocket', socket = App.EventMessageHandler.create()
    socket.connect()

App.loadSettings = ->
  headers = {}
  if App.get('token')
    headers = { Authorization: "Bearer #{App.get 'token'}" }
  req = Em.$.ajax
    type: 'GET'
    url: '/api/v0/app_settings'
    headers: headers
    cache: false
  promise = Em.RSVP.Promise.cast(req)
  promise.then (settings) ->
    App.set 'settings', settings
  promise

App.refreshUsage = ->
  App.BillingUsage.fetchCurrentUsage()

App.refreshBillingProfile = ->
  App.BillingProfile.fetch().then (bp) =>
    App.set 'billingProfile', bp

App.refreshInvoices = ->
  invoices = App.Invoice.fetch().value
  App.set('invoices', invoices)
  invoices

App.loadPlans = ->
  App.set 'eligiblePlans', App.Plan.fetch({ eligible: true }).value

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

$(window).on 'storage', (e) ->
  origEvent = e.originalEvent
  if origEvent.key == "token" && origEvent.newValue == null
    window.location.href = "/app/#logout"

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

  notifyOnboarding: (->
    App.trigger 'handle-transition', @get('url')
  ).on 'didTransition'
