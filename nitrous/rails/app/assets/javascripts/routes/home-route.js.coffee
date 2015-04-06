#= require ./authed-route

App.HomeRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    App.set 'title', null
    hostsFetch = App.Host.fetch()
    containersFetch = App.Container.fetch()
    sharedContainersFetch = App.SharedContainer.fetch()
    App.set 'hosts', hostsFetch.value
    App.set 'containers', containersFetch.value
    App.set 'sharedContainers', sharedContainersFetch.value
    Em.RSVP.allSettled([hostsFetch, containersFetch, sharedContainersFetch])

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'home', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'home', outlet: 'sidebar', controller: 'sidebar'
    @render 'admin/dialogs', into: 'home', outlet: 'admin', controller: 'admin-dialogs'
    @render 'feedback', into: 'home', outlet: 'feedback', controller: 'feedback'

  actions:
    downloadDesktopApp: ->
      Mp.track Mp.EventDashboardUserClickedDownloadDesktopApp
      window.open App.get('desktopDownloadUrl'), '_blank'
