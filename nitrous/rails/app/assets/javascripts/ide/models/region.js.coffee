#= require ./base-model

App.Region = klass = App.BaseModel.extend
  name: Em.attr()
  description: Em.attr()
  state: Em.attr()
  ssl_enabled: Em.attr()

  enabled: (->
    @get('state') != 'down'
  ).property 'state'

  goingDown: (->
    @get('state') == 'going_down'
  ).property 'state'

klass.reopenClass
  rootKey: 'region'
  collectionKey: 'regions'
  url: '/regions'

  fetchClosestRegion: ->
    $.ajax(type: 'GET', dataType: 'json', url: "#{@url}/fetch_closest")
