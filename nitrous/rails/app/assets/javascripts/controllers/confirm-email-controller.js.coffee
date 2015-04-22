App.ConfirmEmailController = Em.Controller.extend
  queryParams: ['token']
  id: null
  token: null

  alreadyConfirmed: false
  confirmLapsed: false
  confirming: false
  generalError: false
  resendingConfirm: false
  resentConfirm: false

  actions:
    resendConfirmationEmail: ->
      @set 'confirming', false
      @set 'alreadyConfirmed', false
      @set 'confirmLapsed', false
      @set 'generalError', false
      @set 'resendingConfirm', true
      @set 'resentConfirm', false

      Em.RSVP.Promise.cast(Em.$.ajax(
        type: 'POST'
        url: "/api/v0/user/resend_confirm?token=#{@get('token')}"
        cache: false
        contentType: false
      )).then (json) =>
        @set 'resendingConfirm', false
        @set 'resentConfirm', true
      , (jqxhr) =>
        @set 'resendingConfirm', false
        @set 'generalError', true

  startConfirm: ->
    @set 'confirming', true
    @set 'alreadyConfirmed', false
    @set 'confirmLapsed', false
    @set 'generalError', false

    Em.RSVP.Promise.cast(Em.$.ajax(
      type: 'POST'
      url: "/api/v0/user/confirm?token=#{@get('token')}"
      cache: false
      contentType: false
    )).then (json) =>
      App.set 'token', json.access_token
      @set 'confirming', false
      App.snackbar.post 'Your email has been confirmed'
      Mp.track Mp.EventUserConfirmedEmail
      Mp.timestamp Mp.PropertyConfirmedEmail
      Em.run.next =>
        @transitionToRoute 'home.index'
    , (jqxhr) =>
      @set 'confirming', false
      if jqxhr.responseJSON.error == "confirmation_expired"
        @set 'confirmLapsed', true
      else
        @set 'generalError', true
