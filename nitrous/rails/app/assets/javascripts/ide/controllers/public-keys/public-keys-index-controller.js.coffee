App.PublicKeysIndexController = Em.ObjectController.extend
  content: (->
    App.PublicKey.all()
  ).property()

  refresh: ->
    @get('content').reload()

  destroy: (publicKey) ->
    publicKey.deleteRecord()
