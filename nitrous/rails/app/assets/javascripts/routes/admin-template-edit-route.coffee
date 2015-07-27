#= require ./admin-route

App.AdminTemplateEditRoute = App.AdminRoute.extend
  currentSlug: null

  activate: ->
    App.set 'title', 'Admin - Edit Template'

  model: (params) ->
    @set 'currentSlug', params.templateSlug
    {} #FIXME: hack since we don't use an actual model

  renderTemplate: ->
    @render 'admin/template-edit', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.set 'currentSlug', @get('currentSlug')
      controller.loadTemplate()
