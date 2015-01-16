#= require ./admin-route

App.ProviderTemplateRoute = App.AdminRoute.extend
  activate: ->
    App.set 'title', 'Admin - Provider Template'

  model: (params) ->
    App.ProviderTemplate.fetch(params.ptid)

  renderTemplate: ->
    @render 'provider-template', outlet: 'content'
    
