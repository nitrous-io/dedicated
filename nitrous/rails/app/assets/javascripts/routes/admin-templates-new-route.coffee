#= require ./admin-route

App.AdminTemplatesNewRoute = App.AdminRoute.extend
  activate: ->
    App.set 'title', 'Admin - New Template'

  renderTemplate: ->
    @render 'admin/templates-new', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
