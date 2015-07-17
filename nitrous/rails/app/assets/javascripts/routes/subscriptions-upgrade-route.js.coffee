App.SubscriptionsUpgradeRoute = App.AuthedRoute.extend

  activate: ->
    App.set 'title', 'Upgrade Subscription'

  setupController: (controller, cxt, transition) ->
    @_super.apply this, arguments
    # Ember routing does not restore some queryParams
    # after re-entering a route
    if (pl = transition?.queryParams?.planId)
      controller.set 'planId', pl
    controller.clear()

  renderTemplate: ->
    @render 'subscriptions/upgrade', outlet: 'content'

  resetController: (controller, isExiting, transition) ->
    if isExiting
      controller.set('planId', null)