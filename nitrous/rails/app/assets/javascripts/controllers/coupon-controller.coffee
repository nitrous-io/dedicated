App.CouponController = Em.Controller.extend
  queryParams: ['code']
  code: null
  saving: false

  actions:
    claim: ->
      @set 'saving', true
      coupon = App.Coupon.create
        code: @get('code')
      coupon.save().then =>
        @set 'saving', false
        App.set('pendingCoupon', null)
        App.snackbar.post("Coupon “#{@get('code')}” claimed successfully.")
        @transitionToRoute 'home'
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to claim coupon!')
        @set 'saving', false
      false
