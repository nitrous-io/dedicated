#= require ./admin-route

App.AdminFreeQueueRoute = App.AdminRoute.extend
  activate: ->
    App.set 'title', 'Admin - Free Queue'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.clear()

  renderTemplate: ->
    @render 'admin/free-queue', outlet: 'content'