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
      mustUpgrade = host.freighter_upgrade_available
      console.log "freighterUpgradeAvailable: ", mustUpgrade
      if !mustUpgrade
        transition.abort()
        @transitionTo "ide", container.slug
        console.log "must upgrade!"
    , =>
      console.log "failed to fetch"

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.upgrade()

  renderTemplate: ->
    @_super()
