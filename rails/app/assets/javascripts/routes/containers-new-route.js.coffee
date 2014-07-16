App.ContainersNewRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'New Container'

  renderTemplate: ->
    @render 'containers/new', outlet: 'content'
