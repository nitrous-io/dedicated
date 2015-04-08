#= require ./authed-route

App.HomeRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    App.set 'title', null
    App.set 'hosts', App.Host.fetch().value
    App.set 'containers', App.Container.fetch().value
    App.set 'sharedContainers', App.SharedContainer.fetch().value

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
