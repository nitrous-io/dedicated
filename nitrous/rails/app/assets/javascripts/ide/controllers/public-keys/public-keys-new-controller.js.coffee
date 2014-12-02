App.PublicKeysNewController = Em.ObjectController.extend
  save: ->
    record = @get 'content'
    record.save().then ->
      App.util.mixpanelIncrement 'ssh_keys_creation_successful_count'
      App.get('router').transitionTo 'public_keys.index'
    , (e) ->
      App.util.mixpanelIncrement 'ssh_keys_creation_failed_count'

  cancel: ->
    App.get('router').transitionTo 'public_keys.index'
