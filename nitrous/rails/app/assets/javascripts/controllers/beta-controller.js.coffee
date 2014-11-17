App.BetaController = Em.Controller.extend
  subject: null
  message: null
  sending: false
  login: null
  switching: false
  closeFeedback: false
  closeSwitchUser: false

  clear: ->
    @setProperties
      subject: null
      message: null
      login: null
      switching: false
      sending: false
      closeFeedback: false
      closeSwitchUser: false

  body: (->
    message = @get 'message'
    userAgent = navigator.userAgent
    platform = navigator.platform
    "User Agent: #{userAgent}\nPlatform: #{platform}\n\n#{message}"
  ).property 'message'

  actions:
    send: ->
      @set 'sending', true

      data =
        subject: @get 'subject'
        body: @get 'body'

      App.User.adapter.ajax('POST', '/feedback', data).then =>
        App.snackbar.post("Your feedback was successfully sent!")
        @clear()
        @set 'closeFeedback', true
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to send feedback!")
        @set 'sending', false

    switchUser: ->
      @set 'switching', true

      data =
        login: @get 'login'

      App.User.adapter.ajax('POST', '/admin/switch_user', data).done((d) =>
        App.set 'token', d.access_token
        @set 'closeSwitchUser', true
        App.snackbar.post("Successfully logged in as #{@get('login')}")

        Em.run.next ->
          location.href = "/"
      ).fail (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to switch user!")
        @set 'switching', false
