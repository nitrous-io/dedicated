App.UserSettingsController = Em.ObjectController.extend
  saving: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      saving: false

  actions:
    save: ->
      @set 'saving', true
      App.get('userProfile').save().then =>
        App.snackbar.post("Account settings saved.")
        @transitionToRoute '/'
        @set 'saving', false
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to connect!')
        @set 'saving', false

    cancel: ->
      @transitionToRoute '/'
