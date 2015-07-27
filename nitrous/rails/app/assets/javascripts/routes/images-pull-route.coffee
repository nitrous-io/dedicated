#= require ./authed-route

App.ImagesPullRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Pull Image'

  renderTemplate: ->
    @render 'images/pull', outlet: 'content'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
