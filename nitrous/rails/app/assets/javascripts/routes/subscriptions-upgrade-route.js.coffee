App.SubscriptionsUpgradeRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments

  activate: ->
    App.set 'title', 'Upgrade Subscription'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.clear()

  renderTemplate: ->
    @render 'subscriptions/upgrade', outlet: 'content'