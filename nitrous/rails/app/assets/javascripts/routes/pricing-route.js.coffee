App.PricingRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Pricing'
    Mp.track Mp.EventViewedPricing

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'pricing', outlet: 'topnav', controller: 'topnav'
