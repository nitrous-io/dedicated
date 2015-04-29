message = 'You will lose unsaved work if you close the IDE.'

App.IdeUpgradeRoute = Em.Route.extend
  beforeModel: ->
    console.log "ideupgrade beforeModel"
    if App.get 'token'
      settingsFetch = App.loadSettings()
      userFetch = App.User.fetch().then (user) ->
        App.set 'user', user
      userSettingsFetch = App.UserSettings.fetch().then (settings) ->
        App.set 'userSettings', settings
      , (jqxhr) ->
        if jqxhr.responseJSON?.error == "outstanding_invoices"
          App.snackbar.post "IDE unavailable while you have outstanding invoices. Redirecting.."
        window.location.href = '/app'
      Em.RSVP.allSettled([settingsFetch, userFetch, userSettingsFetch])
    else
      # redirect to login screen if user hasn't logged in
      window.location.href = '/app/#/login'

  model: (params) ->
    console.log "ideupgrade model"
    @_super.apply this, arguments
    slug = params.containerSlug

    if !slug
      window.location.href = '/404.html'
    else
      App.loadContainer(slug).catch ->
        window.location.href = '/404.html'

  afterModel: (container, transition)->
    console.log "ideupgrade afterModel", container
    hostPromise = container?.loadHost().catch ->
      transition.abort()
    hostPromise.then (resp) =>
      host = resp.host
      # TODO: when upgrade available, do the upgrade
      # console.log host
      needsUpgrade = host.freighter_upgrade_available
      needsUpgrade = true
      console.log "freighterUpgradeAvailable: ", needsUpgrade
      if !needsUpgrade
        transition.abort()
        @transitionTo "ide", container.slug
        console.log "needs upgrade!"
    , =>
      console.log "failed to fetch"

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.upgrade()

  renderTemplate: ->
    @_super()

  activate: ->
    # warn the user before actually unload the window. This is helpful to
    # prevent mis-click close tab / cmd + w
    return if App.get 'redirecting'
    $(window).bind 'beforeunload.ide', (ev)->
      message

  deactivate: ->
    $(window).unbind 'beforeunload.ide'

  actions:
    willTransition: (transition) ->
      return true if App.get 'isChromeWebView'
      return true if App.get 'redirecting'
      return true if window.confirm(message + '\n\nAre you sure you want to leave this page?')
      url = transition.router.recognizer.generate 'box.ide', box_id: this.get('controller.content.id')
      transition.router.updateURL(url)
      transition.abort()
