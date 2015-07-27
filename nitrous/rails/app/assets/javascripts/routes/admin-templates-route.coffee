#= require ./admin-route

App.AdminTemplatesRoute = App.AdminRoute.extend
  activate: ->
    App.set 'title', 'Admin - Templates'

  model: (params) ->
    App.get('templates')?.reload()

  renderTemplate: ->
    @render 'admin/templates', outlet: 'content'
