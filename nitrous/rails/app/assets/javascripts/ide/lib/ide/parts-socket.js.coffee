App.PartsSocket = App.BaseSocket.extend
  name: 'parts'

  serverURIBinding: 'box.box_server_uri'
  serverName: 'rainbow'

  outputStream: null

  didConnect: ->
    @_super()
    boxId = @get 'box.id'
    socket = @get 'socket'

    outputStream = App.PartOutputStream.create()
    @set 'outputStream', outputStream

    socket.send 'start', boxId, (err) =>
      return if err
      @set 'started', true
      @set('startedFirstTime', true) if !@get('startedFirstTime')

    socket.on 'commandOutput', (data) ->
      outputStream.write data

    socket.on 'commandError', (data) ->
      outputStream.write data

    socket.on 'commandComplete', (code) ->
      outputStream.setProperties
        exitStatus: code
        completed: true

  didClose: ->
    @_super()
    @set 'started', false

  disconnect: ->
    @_super()
    @setProperties
      startedFirstTime: false
      started: false
      box: null
      outputStream: null

  list: (callback) ->
    this.send 'list', callback

  search: (term, callback) ->
    this.send 'search', term, callback

  listAll: (callback) ->
    this.send 'listAll', callback

  status: (part, callback) ->
    this.send 'status', part, callback

  install: (part) ->
    this.send 'install', part

  uninstall: (part) ->
    this.send 'uninstall', part

  start: (part) ->
    this.send 'partStart', part

  stop: (part) ->
    this.send 'partStop', part
