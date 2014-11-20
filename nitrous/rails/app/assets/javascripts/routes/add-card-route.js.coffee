App.AddCardRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    # TODO: FFLAG payments
    if !App.get('user.billingEnabled')
      @transitionTo '/'

  activate: ->
    App.set 'title', 'Add Credit Card'

  renderTemplate: ->
    @render 'add-card', outlet: 'content'

  setupController: (controller) ->
     @_super.apply this, arguments
     unless controller.get 'saving'
       controller.clear()
