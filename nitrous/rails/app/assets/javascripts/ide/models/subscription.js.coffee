#= require ./base-model

App.Subscription = klass = App.BaseModel.extend
  state: Em.attr()
  plan_id: Em.attr()

  active: (->
    @get('state') == 'active'
  ).property 'state'

  plan: (->
    App.Plan.findById @get('plan_id')
  ).property 'plan_id'

  changePlan: (planId, callback) ->
    $.ajax(
      url: '/subscription'
      type: 'PUT'
      data: { plan_id: planId }
    ).done( =>
      callback(null)
    ).fail (e) =>
      response = $.parseJSON e.responseText
      errorMessage = response.errors?.base?[0]
      callback errorMessage

  cancel: (callback) ->
    $.ajax(
      url: '/subscription'
      type: 'DELETE'
    ).done( =>
      callback(null)
    ).fail (e) =>
      response = $.parseJSON e.responseText
      errorMessage = response.errors?.base?[0]
      callback errorMessage

klass.reopenClass
  rootKey: 'subscription'

  subscribe: (planId, stripeToken, callback) ->
    unless stripeToken
      return callback(null)

    $.ajax(
      url: '/subscription'
      type: 'POST'
      data: { plan_id: planId, stripe_token: stripeToken }
    ).done( =>
      callback(null)
    ).fail (e) =>
      response = $.parseJSON e.responseText
      errorMessage = response.errors?.base?[0]
      callback errorMessage
