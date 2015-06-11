#= require ./authed-route

App.HomeRoute = App.AuthedRoute.extend
  model: ->
    @_super.apply this, arguments
    App.set 'title', null
    hFetch = App.Host.fetch()
    cFetch = App.Container.fetch()
    sFetch = App.SharedContainer.fetch()
    App.set 'hosts', hFetch.value
    App.set 'containers', cFetch.value
    App.set 'sharedContainers', sFetch.value

    Ember.RSVP.Promise.all [hFetch, sFetch, cFetch]

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'home', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'home', outlet: 'sidebar', controller: 'sidebar'
    @render 'admin/dialogs', into: 'home', outlet: 'admin', controller: 'admin-dialogs'
    @render 'feedback', into: 'home', outlet: 'feedback', controller: 'feedback'
    @render 'onboarding', into: 'home', outlet: 'onboarding', controller: 'onboarding'

  actions:
    downloadDesktopApp: ->
      Mp.track Mp.EventDashboardUserClickedDownloadDesktopApp
      window.open App.get('desktopDownloadUrl'), '_blank'

    upgradePaid: ->
      params = { upgradingFromFree: true }
      if freeHost = App.get('hosts.firstObject')
        params['hostSlug'] = freeHost.get('slug')
      @transitionTo 'add-card', queryParams: params
