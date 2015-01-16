App.PlanView = Em.View.extend
  templateName: 'plan'
  tagName: 'li'

  plan: null
  selectedPlan: null

  isSelected: (->
    @get('selectedPlan') == @get('plan')
  ).property 'selectedPlan', 'plan'

  click: ->
    @set('selectedPlan', @get('plan'))
