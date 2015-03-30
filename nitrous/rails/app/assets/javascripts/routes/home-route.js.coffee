#= require ./authed-route

App.HomeRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    App.set 'title', null
    hostsFetch = App.Host.fetch()
    containersFetch = App.Container.fetch()
    App.set 'hosts', hostsFetch.value
    App.set 'containers', containersFetch.value
    Em.RSVP.allSettled([hostsFetch, containersFetch])

  renderTemplate: ->
    @_super()
    @render 'beta', into: 'home', outlet: 'beta', controller: 'beta'
    @render 'topnav', into: 'home', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'home', outlet: 'sidebar', controller: 'sidebar'
    @render 'admin/dialogs', into: 'home', outlet: 'admin', controller: 'admin-dialogs'
    @render 'feedback', into: 'home', outlet: 'feedback', controller: 'feedback'

  actions:
    downloadDesktopApp: ->
      Mp.track Mp.EventDashboardUserClickedDownloadDesktopApp
      window.open App.get('desktopDownloadUrl'), '_blank'
