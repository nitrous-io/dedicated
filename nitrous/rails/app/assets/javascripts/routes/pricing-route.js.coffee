App.PricingRoute = Em.Route.extend
  model: ->
    Em.A([
      App.Plan.create(slug: 'shift', name: 'Shift', maxContainers: 2, price: 250000, cpu: 1, memory: '1 GB', storage: '20 GB')
      App.Plan.create(slug: 'turbo', name: 'Turbo', maxContainers: 5, price: 500000, cpu: 2, memory: '2 GB', storage: '30 GB', mostPopular: true)
      App.Plan.create(slug: 'nitro', name: 'Nitro', maxContainers: 10, price: 800000, cpu: 2, memory: '4 GB', storage: '50 GB')
    ])

  activate: ->
    App.set 'title', 'Pricing'

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'pricing', outlet: 'topnav', controller: 'topnav'
