#= require ./authed-route

App.CouponRoute = App.AuthedRoute.extend
  beforeModel: (transition) ->
    if !App.get('token')
      App.set('pendingCoupon', transition.queryParams.code)

    @_super.apply this, arguments

  activate: ->
    App.set 'title', 'Claim a Coupon'

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'coupon', outlet: 'topnav', controller: 'topnav'
