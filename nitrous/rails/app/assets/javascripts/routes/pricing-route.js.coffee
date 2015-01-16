App.PricingRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Pricing'

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'pricing', outlet: 'topnav', controller: 'topnav'
