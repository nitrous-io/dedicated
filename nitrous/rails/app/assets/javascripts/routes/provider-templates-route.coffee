#= require ./admin-route

App.ProviderTemplatesRoute = App.AdminRoute.extend
  activate: ->
    App.set 'title', 'Admin - Provider Templates'

  model: ->
    App.ProviderTemplate.fetch()

  renderTemplate: ->
    @render 'provider-templates', outlet: 'content'
