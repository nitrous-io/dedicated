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
  requiresPhone: attr('boolean', as: 'requires_phone', serialize: false)

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

  totalPriceInDollars: (->
    if cp = @get 'price'
      if bp = @get 'basePrice'
        cp = cp + bp
      cpDollars = cp / 100
      if cpDollars != Math.floor(cpDollars)
        cpDollars = cpDollars.toFixed(2)
      cpDollars.toString()
  ).property 'price', 'basePrice'

klass.reopenClass
  url: '/plans'
  primaryKey: 'id'
  collectionKey: 'plans'
