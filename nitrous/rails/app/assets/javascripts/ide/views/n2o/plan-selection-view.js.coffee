App.PlanSelectionView = Em.View.extend
  templateName: 'templates/n2o/plan-selection'

  plansCount: (->
    @get('plans.length')
  ).property 'plans'
