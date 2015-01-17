#= require ./model
attr = SM.attr

App.Plan = klass = App.Model.extend
  id: attr('number', serialize: false)
  slug: attr()
  name: attr()
  maxContainers: attr('number', as: 'max_containers')
  price: attr('number')
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

  priceInDollars: (->
    if price = @get 'price'
      priceDollars = price / 100
      if priceDollars != Math.floor(priceDollars)
        priceDollars = priceDollars.toFixed(2)
      priceDollars.toString()
  ).property 'price'

klass.reopenClass
  url: '/plans'
  primaryKey: 'id'
  collectionKey: 'plans'
