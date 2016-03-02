#= require ./model
attr = SM.attr

App.Plan = klass = App.Model.extend
  id: attr('number', serialize: false)
  hostingModePlanId: attr('number', as: 'hosting_mode_plan_id', serialize: false)

  slug: attr()
  name: attr()
  description: attr()
  price: attr('number')
  basePrice: attr('number', as: 'base_price')
  hourlyPrice: attr('number', as: 'hourly_price')
  cpu: attr()
  memory: attr()
  storage: attr()
  mostPopular: attr('boolean', as: 'most_popular')
  subscription: attr('boolean')
  org: attr('boolean')
  hidden: attr('boolean')
  trialPeriodDays: attr('number', as: 'trial_period_days')

  maxContainers: attr('number', as: 'max_containers')
  maxSnapshots: attr('number', as: 'max_snapshots')

  inputId: (->
    if slug = @get 'slug'
      "plan-#{slug}"
  ).property 'slug'

  hostingModePlan: (->
    if hostingModePlanId = @get('hostingModePlanId')
      App.get('plans').findBy('id', hostingModePlanId)
  ).property 'hostingModePlanId', 'App.plans.@each'

  displayName: (->
    name = @get 'name'
    price = @get 'price'
    cpu = @get 'cpu'
    memory = @get 'memory'
    storage = @get 'storage'
    if name && memory
      "#{name} - #{memory} ram / #{storage} ssd / #{cpu} cpu"
  ).property 'name', 'price', 'cpu', 'memory', 'storage'

  totalPrice: (->
    cp = @getWithDefault 'price', 0
    bp = @getWithDefault 'basePrice', 0
    cp + bp
  ).property 'price', 'basePrice'

  hostingModePrice: (->
    price = 0
    if @get('hostingModePlan')
      price = @get('hostingModePlan.totalPrice') - @get('totalPrice')
    price = 0 if price < 0
    price
  ).property 'totalPrice', 'hostingModePlan.totalPrice'

  totalPriceInDollars: (->
    formatPrice((@getWithDefault 'totalPrice', 0), 2)
  ).property 'totalPrice'

  basePriceInDollars: (->
    formatPrice((@getWithDefault 'basePrice', 0), 2, false)
  ).property 'basePrice'

  hourlyPriceInCents: (->
    formatPrice((@getWithDefault 'hourlyPrice', 0), 3, false)
  ).property 'hourlyPrice'

  # Cost of adding on hosting mode to this plan.
  hostingModePriceInDollars: (->
    formatPrice((@getWithDefault 'hostingModePrice', 0), 2, false)
  ).property 'hostingModePrice'

  isPaidPlan: (->
    return (@get('price') > 0) || (@get('basePrice') > 0)
  ).property 'price', 'basePrice'

  isFreePlan: (->
    return (@get('price') == 0) && (@get('basePrice') == 0)
  ).property 'price', 'basePrice'

  trialAvailable: (->
    # Don't show misleading free trial ribbon on plan if user is no longer eligible.
    return (App.get('billingProfile.trialEligible') && (@get('trialPeriodDays') > 0))
  ).property 'trialPeriodDays', 'App.billingProfile.trialEligible'

formatPrice = (price, digits, ignoreZero=true) ->
  return if price == 0 && ignoreZero
  pd = price / Math.pow(10, digits)
  if pd != Math.floor(pd)
    pd = pd.toFixed(digits)
  pd.toString()

klass.reopenClass
  url: '/plans'
  primaryKey: 'id'
  collectionKey: 'plans'
