App.OrgsController = Em.Controller.extend
  collection: null
  newRecord: null

  init: ->
    @_super.apply this, arguments
    @set 'model', Em.A()
