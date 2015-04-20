App.PromoCodeInfoMixin = Em.Mixin.create
  promoCodeInfo: (->
    promoCode = @get('promoCode')
    if !promoCode
      return null
    info = Em.ObjectProxy.create()
    Em.$.ajax(
      type: 'GET',
      url: "/api/v0/promo_codes/#{promoCode}"
    ).done((data) =>
      if amount = data.credit_amount
        info.set 'content', (amount / 100)
    )
    info
  ).property 'promoCode'
