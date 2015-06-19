#= require ./model
attr = SM.attr

App.Plan = klass = App.Model.extend
  id: attr('number', serialize: false)
  slug: attr()
  name: attr()
  price: attr('number')
  basePrice: attr('number', as: 'base_price')
  hourlyPrice: attr('number', as: 'hourly_price')
  cpu: attr()
  memory: attr()
  storage: attr()
  mostPopular: attr('boolean', as: 'most_popular')
  subscription: attr('boolean')

  maxContainers: attr('number', as: 'max_containers')
  maxSnapshots: attr('number', as: 'max_snapshots')
  cleanPreview: attr('boolean', as: 'clean_preview')
  desktopApp: attr('boolean', as: 'desktop_app')
  ssh: attr('boolean', as: 'ssh')
  customDomain: attr('boolean', as: 'custom_domain')
  customTemplates: attr('boolean', as: 'custom_templates')

  inputId: (->
    if slug = @get 'slug'
      "plan-#{slug}"
  ).property 'slug'

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

  totalPriceInDollars: (->
    formatPrice((@getWithDefault 'totalPrice', 0), 2)
  ).property 'totalPrice'

  basePriceInDollars: (->
    formatPrice((@getWithDefault 'basePrice', 0), 2, false)
  ).property 'basePrice'

  hourlyPriceInCents: (->
    formatPrice((@getWithDefault 'hourlyPrice', 0), 3, false)
  ).property 'hourlyPrice'

  isPaidPlan: (->
    return (@get('price') > 0) || (@get('basePrice') > 0)
  ).property 'price', 'basePrice'

  isFreePlan: (->
    return (@get('price') == 0) && (@get('basePrice') == 0)
  ).property 'price', 'basePrice'

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
