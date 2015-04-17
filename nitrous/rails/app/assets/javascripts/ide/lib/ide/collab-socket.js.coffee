#= require ./evented-socket

App.CollabSocket = App.EventedSocket.extend
  action: 'collab'
  binary: false
  sessions: null

  init: ->
    @_super.apply this, arguments
    @set 'sessions', {}

  extraParams: (->
    if username = App.get('user.username')
      "username=#{username}"
  ).property 'App.user.username'


  handleMessage: (message) ->
    try
      message = JSON.parse message
    return if !message?  # cannot parse json, ignore message

    [filename, event, data] = message
    if filename && event
      @trigger("#{event}|#{filename}", data)
