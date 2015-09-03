App.ActiveFilesMonitor = klass = Em.Object.extend
  collabSocket: null
  username: null

  remoteFiles: null
  localFiles: null

  init: ->
    @_super.apply this, arguments

    collabSocket = @get 'collabSocket'
    collabSocket.on "active|global", @handleActiveReceived.bind(this)
    collabSocket.on "joined|global", @handleJoined.bind(this)
    collabSocket.on "left|global", @handleLeft.bind(this)

    collabSocket.on "reconnected", @handleReconnected.bind(this)

    @set 'remoteFiles', {}
    @set 'localFiles', {}

    # refresh list
    collabSocket.send "", "active"

  handleActiveReceived: (data)->
    hsh = {}
    for id in Object.keys(data.sessions)
      file = id.match(/^[^:]+:(.+)/)[1]
      clients = []
      for cID in Object.keys(data.sessions[id].clients)
        client = data.sessions[id].clients[cID]
        unless client.name == @get("username")
          clients.push client.name
      if clients.length
        hsh[file] = clients
    @set 'remoteFiles', hsh

  handleJoined: (msg)->
    file = msg.namespace.match(/^[^:]+:(.+)/)[1]
    data = msg.data
    remoteFiles = @get 'remoteFiles'
    existing = remoteFiles[file] || []
    existing.push(data.username)
    remoteFiles[file] = existing
    @propertyDidChange 'remoteFiles'

  handleLeft: (msg)->
    file = msg.namespace.match(/^[^:]+:(.+)/)[1]
    data = msg.data
    remoteFiles = @get 'remoteFiles'
    if existing = remoteFiles[file]
      loc = existing.indexOf(data.username)
      if loc != -1
        existing.splice(loc, 1)

        if existing.length == 0
          delete remoteFiles[file]
        @propertyDidChange 'remoteFiles'

  sortedFiles: (->
    remoteFiles = @get 'remoteFiles'
    sortedKeys = Object.keys(remoteFiles).sort()
    sortedKeys.map (k)=>
      {
        name: k,
        clients: remoteFiles[k].sort()
      }
  ).property 'remoteFiles'

  handleReconnected: (data)->
    debugger