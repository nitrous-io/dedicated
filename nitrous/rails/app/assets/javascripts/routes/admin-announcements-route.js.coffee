#= require ./admin-route

App.AdminAnnouncementsRoute = App.AdminRoute.extend
  activate: ->
    App.set 'title', 'Admin - Announcements'

  renderTemplate: ->
    @render 'admin/announcements', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.clear()
