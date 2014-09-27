App.BetaController = Em.Controller.extend
  subject: null
  message: null
  sending: false
  close: false

  clear: ->
    @setProperties
      subject: null
      message: null
      sending: false
      close: false

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
        App.snackbar.post("Your feedback has been succesfully sent!")
        @clear()
        @set 'close', true
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to send feedback!")
        @set 'sending', false
