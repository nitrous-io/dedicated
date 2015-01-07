#= require ../../../lib/socket/socket-connection

App.OpsSocket = Em.Object.extend
  conn: null
  container: null
  started: false
  startedFirstTime: false
  startedAt: null

  nextCallbackId: 0
  callbacks: []

  init: ->
    if container = @get 'container'
      conn = new SocketConnection()
      conn.onopen    = (ev) => @didOpen(ev)
      conn.onmessage = (ev) => @didReceiveMessage(ev)
      conn.onerror   = (ev) => @didFail(ev)
      conn.onclose   = (ev) => @didClose(ev)
      @set 'conn', conn
      @set 'nextCallbackId', 0
      @set 'callbacks', []

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

  open: ->
    container = @get 'container'
    conn = @get('conn')
    if container && conn
      protocol = location.protocol.replace('http', 'ws')
      freighterdUrl = container.get 'freighterdUrl'
      cid = container.get 'cid'
      token = container.get 'token'

      conn.url = "#{protocol}//#{freighterdUrl}/containers/#{cid}/ops?token=#{token}"
      conn.open(true)
      @set 'startedAt', Date.now()

  didOpen: ->
    @set 'started', true
    @set('startedFirstTime', true) if !@get('startedFirstTime')
    # TODO re-enable timing tracking later
    #duration = Date.now() - @get('startedAt')
    #App.TimingMetrics.record 'sftp_connection', duration, box_id: @get('container.id')

  close: ->
    @get('conn')?.close()
    @setProperties
      startedFirstTime: false
      started: false
      startedAt: null
      container: null

  send: (namespace, event, args...) ->
    if conn = @get('conn')
      if _.isFunction(_.last args)
        callback = args.pop()
        callbackId = @registerCallback callback
        msg = { namespace: namespace, event: event, cid: callbackId, args: args }
        conn.send "d|#{JSON.stringify(msg)}"
      else
        msg = { namespace: namespace, event: event, args: args }
        conn.send "d|#{JSON.stringify(msg)}"

  didReceiveMessage: (event) ->
    type = event.data.slice(0, 2)
    switch type
      when 'd|'
        @handleMessage(event.data.slice(2))
      when 'c|'
        @close()
      when 'h|'
        # no-op
        return

  handleMessage: (message) ->
    try
      message = JSON.parse message
    if !message.error
      @invokeCallback message.cid, [null].concat(message.args)
    else
      # TODO test error handling
      @invokeCallback message.cid, [message.error]

  didFail: (event) ->
    # TODO: implement

  didClose: ->
    @set 'started', false

  fetchFileList: (absolutePath, callback) ->
    @send 'fs', 'list', absolutePath, callback

  fetchFileContent: (absolutePath, callback) ->
    @send 'fs', 'read', absolutePath, callback

  downloadFile: (absolutePath, callback) ->
    @send 'fs', 'download', absolutePath, callback

  writeFile: (absolutePath, content, callback) ->
    @send 'fs', 'write', absolutePath, content, callback

  writeNewFile: (absolutePath, content, callback) ->
    @send 'fs', 'writeNew', absolutePath, content, callback

  makeDirectory: (absolutePath, callback) ->
    @send 'fs', 'makeDir', absolutePath, callback

  sendUploadToken: (absolutePath, token, callback) ->
    @send 'fs', 'upload', absolutePath, token, callback

  deleteFile: (absolutePath, callback) ->
    @send 'fs', 'deleteFile', absolutePath, callback

  deleteDir: (absolutePath, callback) ->
    @send 'fs', 'deleteDir', absolutePath, callback

  rename: (currentPath, newPath, callback) ->
    @send 'fs', 'rename', currentPath, newPath, callback
