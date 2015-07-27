App.SubscriptionsRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments

  model: (params) ->
    App.get('subscriptions').findBy('slug', params.slug)