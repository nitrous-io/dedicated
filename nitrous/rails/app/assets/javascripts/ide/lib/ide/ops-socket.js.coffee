#= require ../../../lib/socket/socket-connection
#= require ../../../lib/socket/socket

App.OpsSocket = Lib.Socket.extend
  container: null
  started: false
  startedFirstTime: false

  nextCallbackId: 0
  callbacks: null

  init: ->
    @_super.apply this, arguments
    @set 'nextCallbackId', 0
    @set 'callbacks', []

  connect: ->
    return unless @get 'container'
    @_super.apply this, arguments

  disconnect: ->
    @_super.apply this, arguments
    @setProperties
      startedFirstTime: false
      started: false
      container: null

  send: (event, args...) ->
    return unless @_conn
    if _.isFunction(_.last args)
      callback = args.pop()
      callbackId = @registerCallback callback
      msg = { namespace: "fs", event: event, cid: callbackId, args: args }
      @_conn.send "d|#{JSON.stringify(msg)}"
    else
      msg = { namespace: "fs", event: event, args: args }
      @_conn.send "d|#{JSON.stringify(msg)}"

  willOpen: (ev) ->
    container = @get 'container'
    protocol = location.protocol.replace('http', 'ws')
    freighterdUrl = container.get 'freighterdUrl'
    cid = container.get 'cid'
    token = container.get 'token'
    @set 'url', "#{protocol}//#{freighterdUrl}/containers/#{cid}/ops?token=#{token}"
    @_super.apply this, arguments

  didOpen: ->
    @_super.apply this, arguments
    @set 'started', true
    @set('startedFirstTime', true) if !@get('startedFirstTime')

  didReceiveMessage: (event) ->
    @_super.apply this, arguments
    type = event.data.slice(0, 2)
    switch type
      when 'd|'
        @handleMessage(event.data.slice(2))
      when 'c|'
        @disconnect()
      when 'h|'
        # no-op
        return

  didClose: ->
    @_super.apply this, arguments
    @set 'started', false

  registerCallback: (callback) ->
    callbackId = @get 'nextCallbackId'
    @get('callbacks')[callbackId] = callback
    @incrementProperty('nextCallbackId')
    callbackId

  invokeCallback: (callbackId, args) ->
    callbacks = @get('callbacks')
    if callback = callbacks[callbackId]
      callbacks[callbackId] = null
      callback args...

  handleMessage: (message) ->
    try
      message = JSON.parse message
    if !message.error
      @invokeCallback message.cid, [null].concat(message.args)
    else
      # TODO test error handling
      @invokeCallback message.cid, [message.error]

  fetchFileList: (absolutePath, callback) ->
    @send 'list', absolutePath, callback

  fetchFileContent: (absolutePath, callback) ->
    @send 'read', absolutePath, callback

  downloadFile: (absolutePath, callback) ->
    @send 'download', absolutePath, callback

  writeFile: (absolutePath, content, callback) ->
    @send 'write', absolutePath, content, callback

  writeNewFile: (absolutePath, content, callback) ->
    @send 'writeNew', absolutePath, content, callback

  makeDirectory: (absolutePath, callback) ->
    @send 'makeDir', absolutePath, callback

  sendUploadToken: (absolutePath, token, callback) ->
    @send 'upload', absolutePath, token, callback

  deleteFile: (absolutePath, callback) ->
    @send 'deleteFile', absolutePath, callback

  deleteDir: (absolutePath, callback) ->
    @send 'deleteDir', absolutePath, callback

  rename: (currentPath, newPath, callback) ->
    @send 'rename', currentPath, newPath, callback
