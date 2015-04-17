#= require ./evented-socket

App.CollabSocket = klass = App.EventedSocket.extend
  action: 'collab'
  binary: false
  connID: null

  extraParams: (->
    params = []
    if username = App.get('user.username')
      params.push "username=#{username}"
    if connID = @get 'connID'
      params.push "conn_id=#{connID}"
    params.join('&')
  ).property 'App.user.username', 'connID'

  handleMessage: (message) ->
    try
      message = JSON.parse message
    return if !message?  # cannot parse json, ignore message

    [filename, event, data] = message

    if event == 'connected'
      if @get('connID')
        # connID is already set, it is a reconnect
        @trigger 'reconnected'
      @set 'connID', data.conn_id
      return

    if filename && event
      @trigger("#{event}|#{filename}", data)

klass.reopenClass
  checkAvailability: (container, cb) ->
    protocol = location.protocol
    freighterdUrl = container.get 'freighterdUrl'
    url = "#{protocol}//#{freighterdUrl}/features"
    Em.$.ajax(
      type: 'GET'
      url: url
    ).done((data) =>
      if data.features && data.features.indexOf('collab') != -1
        cb(true)
      else
        cb(false)
    ).fail (jqxhr) =>
      cb(false)
