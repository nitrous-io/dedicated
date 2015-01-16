#= require ./model
attr = SM.attr

App.ProviderTemplate =  klass = App.Model.extend
  id: attr('number')
  name: attr('string')
  regionId: attr('number', as: 'region_id')
  planId: attr('number', as: 'plan_id')
  provider: attr('string')
  hourlyPrice: attr('number', as: 'hourly_price')
  preprovisionMax: attr('number', as: 'preprovision_max')

  # Specifying a random attribute type
  # Because we need the original json object
  # unconverted. But having unknown attr type
  # is the only way to go for it
  config: attr('object')
  
  createdAt: attr('date', as: 'created_at')
  updatedAt: attr('date', as: 'updated_at')

  configString: (->
    config = @get 'config'
    JSON.stringify(config, null, 2)
  ).property('config')

  save: (param, options) ->
    ptid = @get 'id'
    
    json = provider_template:
      id: @get 'id'
      preprovision_max: parseInt(@get 'preprovisionMax') || 0
      hourly_price: parseInt(@get 'hourlyPrice') || 0
      config: @get 'config'

    @ajax('POST', '', JSON.stringify(json), 'content-type': 'application/json')

klass.reopenClass
  url: '/admin/provider_templates'
  primaryKey: 'provider_template'
  rootKey: 'provider_template'
  collectionKey: 'provider_templates'