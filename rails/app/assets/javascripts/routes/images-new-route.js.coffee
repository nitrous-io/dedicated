App.ImagesNewRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'New Image'

  renderTemplate: ->
    @render 'images/new', outlet: 'content'
