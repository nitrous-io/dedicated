App.AccountController = Em.ObjectController.extend
  saving: false
  updatingDetails: false

  advanced: false

  email: null
  password: null
  confirmPassword: null

  detailsInvalidField: null
  detailsError: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      saving: false
      advanced: App.get('userProfile.advanced')
      updatingDetails: false
      email: App.get('user.email')
      password: null
      confirmPassword: null

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

    updateDetails: ->
      email = @get 'email'
      @set 'updatingDetails', true
      @set 'detailsError', false
      @set 'detailsInvalidField', null

      unless @get('email')?.length > 0
        App.snackbar.post("Email must be provided.")
        @set 'detailsInvalidField', 'email'
        @set 'detailsError', true
        @set 'updatingDetails', false
        return

      if @get('password')?.length > 0 && @get('password') != @get('confirmPassword')
        App.snackbar.post("Password doesn't match the confirmation.")
        @set 'detailsInvalidField', 'password'
        @set 'detailsError', true
        @set 'updatingDetails', false
        return

      curUser = App.get('user')
      curUser.set('email', @get('email'))

      if @get('password')?.length > 0
        curUser.set('password', @get('password'))

      curUser.forceUpdate().then =>
        App.snackbar.post("Account details updated.")
        @transitionToRoute '/'
        @set 'updatingDetails', false
      , (jqxhr) =>
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'detailsInvalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to update details')
        @set 'updatingDetails', false