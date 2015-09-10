App.ActiveFilesMonitor = klass = Em.Object.extend
  collabSocket: null
  username: null

  remoteFiles: null
  localFiles: null

  init: ->
    @_super.apply this, arguments

    collabSocket = @get 'collabSocket'
    collabSocket.on "activefiles|global", @handleActiveReceived.bind(this)
    collabSocket.on "joined|global", @handleJoined.bind(this)
    collabSocket.on "left|global", @handleLeft.bind(this)

    collabSocket.on "reconnected", @handleReconnected.bind(this)

    @set 'remoteFiles', {}
    @set 'localFiles', {}
    @userMap = {}

    # refresh list
    @refresh()

  tabsDidChange: (tabs)->
    hsh = {}
    for tab in tabs.get('content')
      pathObj = tab.get && tab.get('path')
      if pathObj.get && (pth = pathObj.get('absolutePath'))
        hsh[pth] = true

    @set 'localFiles', hsh
    @propertyDidChange 'localFiles'

  handleActiveReceived: (data)->
    hsh = {}
    for id in Object.keys(data.sessions)
      file = @fileFromNamespace(id)
      clients = []
      for cID in Object.keys(data.sessions[id].clients)
        client = data.sessions[id].clients[cID]
        @userMap[cID] = client.name

        unless client.name == @get("username")
          clients.push cID
      if clients.length
        hsh[file] = clients
    @set 'remoteFiles', hsh

  handleJoined: (msg)->
    data = msg.data
    return if data.username == @get("username")

    @userMap[data.conn_id] = data.username

    file = @fileFromNamespace(msg.namespace)
    remoteFiles = @get 'remoteFiles'
    existing = remoteFiles[file] || []
    existing.push data.conn_id
    remoteFiles[file] = existing

    @propertyDidChange 'remoteFiles'

  handleLeft: (msg)->
    data = msg.data
    return if data.username == @get("username")

    @userMap[data.conn_id] = data.username

    file = @fileFromNamespace(msg.namespace)
    remoteFiles = @get 'remoteFiles'
    if existing = remoteFiles[file]
      loc = existing.indexOf data.conn_id
      if loc != -1
        existing.splice(loc, 1)

        if existing.length == 0
          delete remoteFiles[file]
        @propertyDidChange 'remoteFiles'

  sortedFiles: (->
    localFiles = @get 'localFiles'
    remoteFiles = @get 'remoteFiles'

    allKeys = Object.keys(localFiles)
    for key in Object.keys(remoteFiles)
      if allKeys.indexOf(key) == -1
        allKeys.push key

    sortedKeys = allKeys.sort()
    sortedKeys.map (k)=>
      hasRemoteClients = false
      connIDs = []
      openedLocal = localFiles[k] || false
      if openedLocal
        connIDs.push @get 'collabSocket.connID'
      if (remote = remoteFiles[k])?.length
        hasRemoteClients = true
        connIDs = connIDs.concat remote

      clients = []
      for cID in connIDs.sort().reverse()
        uname = if cID == @get 'collabSocket.connID'
          @get 'username'
        else
          @userMap[cID]

        if clients.indexOf(uname) == -1
          clients.push(uname)

      {
        path: App.Path.create(absolutePath: k),
        clients: clients,
        openedLocal: openedLocal,
        hasRemoteClients: hasRemoteClients
      }
  ).property 'remoteFiles', 'localFiles'

  handleReconnected: ->
    @refresh()

  refresh: ->
    collabSocket = @get 'collabSocket'
    collabSocket.send "", "activefiles"

  fileFromNamespace: (namespace)->
    namespace.match(/^[^:]+:(.+)/)[1]
