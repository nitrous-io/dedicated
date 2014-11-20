App.AddCardRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Add Credit Card'

  renderTemplate: ->
    @render 'add-card', outlet: 'content'

  setupController: (controller) ->
     @_super.apply this, arguments
     unless controller.get 'saving'
       controller.clear()
