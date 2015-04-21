#= require ./admin-route

App.AdminFreeQueueRoute = App.AdminRoute.extend
  activate: ->
    App.set 'title', 'Admin - Free Queue'

  renderTemplate: ->
    @render 'admin/free-queue', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.clear()
