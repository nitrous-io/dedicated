# Overrides sections of BaseSocket to support multiple namespaces within a single
# socket.  PartsSocket should eventually be moved into here.

IdDataDelimiter = "!!\n"

App.CommandSocket = App.BaseSocket.extend
  name: 'command'
  additionalNamespaces: ['search', 'filefind']

  serverURIBinding: 'box.box_server_uri'
  serverName: 'rainbow'

  commands: null

  init: ->
    @_super()
    @set 'commands', Ember.Object.create()
    @set 'commands.search', App.SearchCommand.create()
    @set 'commands.filefind', App.FileFindCommand.create()

  connect: ->
    @_super()
    _.each this.additionalNamespaces, (namespace) =>
      @connection.on namespace, (action, args) =>
        @socket.emit [namespace, action].join('.'), args...

  didConnect: ->
    @_super()
    boxId = @get 'box.id'
    socket = @get 'socket'

    socket.send 'start', boxId, (err) =>
      return if err
      @set 'started', true

    _.each _.values(@get('commands')), (cmd) ->
      cmd.bindEvents(socket)

  didClose: ->
    @_super()
    @set 'started', false

  disconnect: ->
    if connection = @get 'connection'
      connection.destroy()
    @didClose()
    @setProperties
      socket: null
      connection: null

  _didAuthenticate: ->
    @_super()
    if socket = @get('socket')
      _.each this.additionalNamespaces, (namespace) =>
        socket.sendAs namespace, 'init', -> {}

  sendAs: ->
    if socket = @get 'socket'
      socket.sendAs _.toArray(arguments)...

  search: ->
    @get('commands.search')?.send(this, _.toArray(arguments)...)

  filefind: ->
    @get('commands.filefind')?.send(this, _.toArray(arguments)...)
