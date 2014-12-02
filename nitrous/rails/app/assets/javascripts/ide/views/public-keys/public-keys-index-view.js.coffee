App.PublicKeysIndexView = Em.View.extend
  templateName: 'templates/public-keys/index'
  classNames: ['app_page', 'index']
  elementId: 'public_keys_index_view'

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Public Keys Index'

  didClickDelete: (publicKey) ->
    @get('controller').destroy publicKey

  didClickAddKey: ->
    App.get('router').transitionTo 'public_keys.new'

  didClickRefresh: ->
    @get('controller').refresh()
