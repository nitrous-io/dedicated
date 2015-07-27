#= require ./authed-route

App.ImagesNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Image'

  renderTemplate: ->
    @render 'images/new', outlet: 'content'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
    Mp.track Mp.EventDashboardViewedNewImage
