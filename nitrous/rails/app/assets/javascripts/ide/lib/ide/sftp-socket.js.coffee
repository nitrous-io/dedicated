App.SftpSocket = App.BaseSocket.extend
  name: 'sftp'
  box: null
  started: false
  startedFirstTime: false
  startedAt: null

  serverURIBinding: 'box.box_server_uri'
  serverName: 'rainbow'

  connect: ->
    @_super()
    @set 'startedAt', Date.now()

  didConnect: ->
    @_super()
    boxId = @get 'box.id'
    socket = @get 'socket'

    socket.send 'start', boxId, =>
      @set 'started', true
      @set('startedFirstTime', true) if !@get('startedFirstTime')
      duration = Date.now() - @get('startedAt')
      id       = @get('box.id')

      App.TimingMetrics.record 'sftp_connection', duration, box_id: id

  didClose: ->
    @_super()
    @set 'started', false

  disconnect: ->
    @_super()
    @setProperties
      startedFirstTime: false
      started: false
      startedAt: null
      box: null

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
