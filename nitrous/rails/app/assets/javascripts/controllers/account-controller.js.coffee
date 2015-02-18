App.AccountController = Em.ObjectController.extend
  queryParams: ['errmsg']
  errmsg: null

  updatingSettings: false
  updatingDetails: false

  advanced: false

  name: null
  email: null
  originalEmail: null
  unconfirmedEmail: null
  password: null
  confirmPassword: null

  detailsInvalidField: null
  detailsError: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      updatingSettings: false
      advanced: App.get('userProfile.advanced')
      updatingDetails: false
      name: App.get('user.name')
      email: App.get('user.unconfirmedEmail') || App.get('user.email')
      originalEmail: App.get('user.email')
      unconfirmedEmail: App.get('user.unconfirmedEmail')
      password: null
      confirmPassword: null
    msg = @get('errmsg')
    if msg
      Em.run.later(=>
        Em.run.scheduleOnce 'afterRender', =>
          App.postError "#{msg}"
          @set 'errmsg', null
      , 100)

  noclick: ->
    false

  githubAuthorize: ->
    userProfile = App.get 'userProfile'
    userProfile.githubAuthorize()

  actions:
    updateSettings: ->
      @set 'updatingSettings', true
      userProfile = App.get 'userProfile'
      userProfile.ajax('POST', null,
        advanced: @get 'advanced'
      ).then (data) =>
        App.snackbar.post("Account settings saved.")
        userProfile.setProperties(data.user_profile)
        @set 'updatingSettings', false
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to connect!')
        @set 'updatingSettings', false

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
      curUser.set('name', @get('name'))
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
