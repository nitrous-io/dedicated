#= require ./base-model

App.Plan = klass = App.BaseModel.extend
  name: Em.attr()
  price: Em.attr(Number)
  interval: Em.attr()
  highlight: Em.attr()
  display_order: Em.attr(Number)
  n2o_amount: Em.attr(Number)
  features: Em.attr()
  trial_months: Em.attr()

  formattedPrice: (->
    App.util.asCurrency @get('price')
  ).property 'price'

  trialMonthsWithUnit: (->
    switch @get 'trial_months'
      when 0
        ''
      when 1
        '1 month'
      else
        "#{@get 'trial_months'} months"
  ).property 'trial_months'

klass.reopenClass
  adapter: null
  rootKey: 'plan'

  personalPlans: null
  teamPlans: null

klass.Collection = Em.ArrayProxy.extend Em.SortableMixin,
  sortProperties: ['display_order']
