#= require ./admin-route

App.AdminAnnouncementsNewRoute = App.AdminRoute.extend
  activate: ->
    App.set 'title', 'Admin - New Announcement'

  renderTemplate: ->
    @render 'admin/announcements-new', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.clear()
