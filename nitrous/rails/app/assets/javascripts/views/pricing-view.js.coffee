App.PricingView = Em.View.extend
  elementId: 'pricing-view'

  actions:
    selectPlan: (plan) ->
      @get('controller').transitionToRoute 'signup', queryParams: { plan: plan.get('slug') }
