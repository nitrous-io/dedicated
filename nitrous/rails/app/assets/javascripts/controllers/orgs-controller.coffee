App.OrgsController = Em.Controller.extend
  collection: null
  newRecord: null

  init: ->
    @_super.apply this, arguments
    @set 'collection', []
    PubSub.subscribe 'orgs-new.created', (msg, data) =>
      @get('collection').addObject(data)

  reload: ->
    App.Org.fetch(@get 'collection')
