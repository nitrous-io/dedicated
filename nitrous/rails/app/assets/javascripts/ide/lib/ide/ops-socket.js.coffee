#= require ../../../lib/socket/socket

App.OpsSocket = Lib.Socket.extend Ember.Evented,
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

  send: (namespace, event, args...) ->
    return unless @_conn
    if _.isFunction(_.last args)
      callback = args.pop()
      callbackId = @registerCallback callback
      msg = { namespace: namespace, event: event, cid: callbackId, args: args }
      @_conn.send "d|#{JSON.stringify(msg)}"
    else
      msg = { namespace: namespace, event: event, args: args }
      @_conn.send "d|#{JSON.stringify(msg)}"

  willOpen: (ev) ->
    @set 'url', null
    @_super.apply this, arguments
    container = @get 'container'
    protocol = location.protocol.replace('http', 'ws')
    freighterdUrl = container.get 'freighterdUrl'
    cid = container.get 'cid'

    container.requestToken(['ops']).then (token) =>
      token = encodeURIComponent(token.token)
      @set 'url', "#{protocol}//#{freighterdUrl}/containers/#{cid}/ops?token=#{token}&binary=true"
      @_conn.binaryMode = true
      # proceed with connection
      @connect(true)
    , (jqxhr) =>
      @retry()

  didOpen: (ev) ->
    @_super.apply this, arguments
    @set 'started', true
    @set('startedFirstTime', true) if !@get('startedFirstTime')

  didReceiveMessage: (event) ->
    @_super.apply this, arguments
    type = event.data.slice(0, 2)
    switch type
      when 'd|'
        @handleMessage(event.data.slice(2))
      when 'h'
        @_conn.send('h')
        return

  didClose: ->
    @_super.apply this, arguments
    if callbacks = @get 'callbacks'
      callbacks.forEach (o, i) =>
        @invokeCallback(i, ['aio:connection:closed'])
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
    return if !message?  # cannot parse json, ignore message

    # check if there are any registered handlers
    # for the incoming event
    # otherwise, try to find corresponding callback
    if this.has(message.event)
      this.trigger message.event, [message.error].concat(message.args)...
    else
      if message.error
        @invokeCallback message.cid, [message.error]
      else
        @invokeCallback message.cid, [null].concat(message.args)

  unwatchDir: (absolutePath) ->
    @send 'fs', 'unwatchDir', absolutePath

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

  fileFind: (term, root, callback) ->
    @send 'finder', 'file_find', term, root, callback

  getFeatures: (callback) ->
    @send 'features', 'get', callback

  # cmd : command to execute as a list e.g. ["ls"], ["ls", "-lah"]
  # cwd : current working directory e.g. "/home". empty strings means whatever
  #       is current in cops process
  # env : additional env variables as a list e.g. ["FOO=bar", "SECOND=two"].
  #       empty list means no additions
  # callback is: (err, cmdErrMsg, cmdStdout, cmdStderr)
  # Note: the command is run with shell, which includes things like substitution
  # of shell variables e.g. command ["echo", "$PATH"] will print PATH env variable
  run: (cmd, cwd, env, callback) ->
    @send 'cmd', 'run', cmd, cwd, env, callback

  search: (id, term, location, ignoreCase, useRegexp, filePattern, callback) ->
    request =
      id: id
      term: term
      root: location
      ignore_case: ignoreCase
      use_regexp: useRegexp
      file_pattern: filePattern
    @send 'finder', 'search', request, callback
