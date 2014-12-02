#= require ./base-model

App.BillingProfile = klass = App.BaseModel.extend
  formatted_card_number: Em.attr()
  card_type: Em.attr()

  changeCard: (stripeToken, callback) ->
    unless stripeToken
      return callback(null)

    $.ajax(
      url: '/subscription/change_card'
      type: 'PUT'
      data: { stripe_token: stripeToken }
    ).done( =>
      callback(null)
    ).fail (e) =>
      response = $.parseJSON e.responseText
      errorMessage = response.errors?.base?[0]
      callback errorMessage

klass.reopenClass
  rootKey: 'billing_profile'
