App.N2oChangePlanView = Em.View.extend
  templateName: 'templates/n2o/change-plan'
  classNames: ['app_page']
  elementId: 'n2o_change_plan_view'

  changePlanDisabled: (->
    controller = @get 'controller'

    # Disable when processing
    return true if controller.get('processing')

    # Enable only if user changes plans
    return controller.get('currentPlan') == controller.get('selectedPlan')
  ).property 'controller.processing', 'controller.currentPlan', 'controller.selectedPlan'

  didClickChangePlan: ->
    controller = @get 'controller'
    controller.changePlan()

  didClickChangeCreditCard: ->
    @get('controller').changeCreditCard()
