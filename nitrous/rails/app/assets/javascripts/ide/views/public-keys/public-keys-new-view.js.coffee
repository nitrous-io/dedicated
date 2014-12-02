App.PublicKeysNewView = Em.View.extend
  templateName: 'templates/public-keys/new'
  classNames: ['app_page', 'new']
  elementId: 'public_keys_new_view'

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Public Keys New'

  didClickAddKey: ->
    @get('controller').save()

  didClickCancel: ->
    @get('controller').cancel()
