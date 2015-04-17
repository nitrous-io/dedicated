#= require ./evented-socket

App.CollabSocket = App.EventedSocket.extend
  action: 'collab'
  binary: false
  connID: null
  isReconnect: false

  extraParams: (->
    params = []
    if username = App.get('user.username')
      params.push "username=#{username}"
    if connID = @get 'connID'
      params.push "conn_id=#{connID}"
    params.join('&')
  ).property 'App.user.username', 'connID'

  didClose: (ev) ->
    @set 'isReconnect', true
    @_super.apply this, arguments

  handleMessage: (message) ->
    try
      message = JSON.parse message
    return if !message?  # cannot parse json, ignore message

    [filename, event, data] = message

    if event == 'connected'
      @set 'connID', data.conn_id

      if @get 'isReconnect'
        @trigger 'reconnect'

    if filename && event
      @trigger("#{event}|#{filename}", data)
