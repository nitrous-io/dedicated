App.PlanView = Em.View.extend
  templateName: 'templates/n2o/plan'
  tagName: 'li'
  classNameBindings: ['selected', 'spanClass']

  spanClass: (->
    plansCount = @get('parentView.plansCount')
    "span#{12 / plansCount}"
  ).property 'parentView.plansCount'

  selected: (->
    controller = @get 'controller'
    selectedPlan = controller.get('selectedPlan')
    plan = @get 'plan'

    selectedPlan && (selectedPlan.get('id') == plan.get('id'))
  ).property 'controller.selectedPlan'

  isCurrentPlan: (->
    controller = @get 'controller'
    currentPlan = controller.get 'currentPlan'
    plan = @get 'plan'

    currentPlan && (currentPlan.get('id') == plan.get('id'))
  ).property 'controller.currentPlan'

  discounted: (->
    coupon = App.currentUser.get('coupon')
    return false unless coupon
    return false if App.get('subscription.active')

    return coupon.percent_off && coupon.percent_off > 0
  ).property 'App.currentUser.coupon', 'App.subscription'

  discount: (->
    coupon = App.currentUser.get('coupon')
    return 0 unless coupon

    percentOff = coupon.percent_off
    price = @get('plan.price')

    price * ( percentOff / 100 )
  ).property 'App.currentUser.coupon', 'plan.price'

  formattedPrice: (->
    "$ #{@get('plan.price').toFixed(2)}"
  ).property 'plan.price'

  formattedTotal: (->
    price = @get('plan.price')
    total = price - @get('discount')

    "$ #{total.toFixed(2)}"
  ).property 'discount', 'plan.price'

  click: ->
    controller = @get 'controller'
    plan = @get 'plan'
    controller.set 'selectedPlan', plan
