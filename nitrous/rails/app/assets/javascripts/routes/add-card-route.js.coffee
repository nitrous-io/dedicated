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

  resetController: (controller, isExiting, transition) ->
    if isExiting
      # Reset query params so they're not sticky.
      controller.set('upgradingFromFree', false)
      controller.set('hostSlug', null)
