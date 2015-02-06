App.AddCardRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments

  activate: ->
    App.set 'title', 'Billing'

  renderTemplate: ->
    @render 'add-card', outlet: 'content'

  setupController: (controller) ->
     @_super.apply this, arguments
     unless controller.get 'saving'
       controller.clear()
