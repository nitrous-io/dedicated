App.ChangeCreditCardDialog = Em.Object.extend
  card: null
  errorMessage: null

  updateCard: (callback) ->
    billingProfile = App.get 'billingProfile'
    card = @get 'card'

    @set 'errorMessage', null

    card.createToken (error, stripeToken) =>
      if error
        @set 'errorMessage', error
        callback(error)
      else
        billingProfile.changeCard stripeToken, (error) =>
          if error
            @set 'errorMessage', error
          else
            card.reset()
          callback(error)
