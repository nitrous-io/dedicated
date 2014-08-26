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

  inputId: (->
    if slug = @get 'slug'
      "plan-#{slug}"
  ).property 'slug'

klass.reopenClass
  url: '/plans'
  primaryKey: 'id'
  collectionKey: 'plans'
