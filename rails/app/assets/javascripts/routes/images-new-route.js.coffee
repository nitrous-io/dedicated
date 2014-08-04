App.ImagesNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Image'

  renderTemplate: ->
    @render 'images/new', outlet: 'content'
