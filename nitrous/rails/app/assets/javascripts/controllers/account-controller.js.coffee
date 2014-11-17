App.AccountController = Em.ObjectController.extend
  saving: false

  advanced: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      saving: false
      advanced: App.get('userProfile.advanced')

  actions:
    save: ->
      @set 'saving', true
      userProfile = App.get 'userProfile'
      userProfile.ajax('POST', null,
        advanced: @get 'advanced'
      ).then (data) =>
        App.snackbar.post("Account settings saved.")
        userProfile.setProperties(data.user_profile)
        @set 'saving', false
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to connect!')
        @set 'saving', false
