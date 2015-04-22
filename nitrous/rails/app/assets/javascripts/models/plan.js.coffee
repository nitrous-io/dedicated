#= require ./model
attr = SM.attr

App.Plan = klass = App.Model.extend
  id: attr('number', serialize: false)
  slug: attr()
  name: attr()
  maxContainers: attr('number', as: 'max_containers')
  price: attr('number')
  basePrice: attr('number', as: 'base_price')
  cpu: attr()
  memory: attr()
  storage: attr()
  mostPopular: attr('boolean', as: 'most_popular')

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
    tp = @getWithDefault 'totalPrice', 0
    return if tp == 0
    tpd = tp / 100
    if tpd != Math.floor(tpd)
      tpd = tpd.toFixed(2)
    tpd.toString()
  ).property 'totalPrice'

  isPaidPlan: (->
    return (@get('price') > 0) || (@get('basePrice') > 0)
  ).property 'price', 'basePrice'

  isFreePlan: (->
    return (@get('price') == 0) && (@get('basePrice') == 0)
  ).property 'price', 'basePrice'

klass.reopenClass
  url: '/plans'
  primaryKey: 'id'
  collectionKey: 'plans'
