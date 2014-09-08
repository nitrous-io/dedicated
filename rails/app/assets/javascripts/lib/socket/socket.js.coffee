Lib.Socket = klass = Em.Object.extend Em.Evented,
  url: null

  connect: ->
    url = @get 'url'
    protocol = location.protocol.replace 'http', 'ws'
    @_conn = conn = new WebSocket "#{protocol}//#{url}" # not using @set to reduce overhead
    conn.onopen    = (ev) => @_didOpen(ev)
    conn.onmessage = (ev) => @_didReceiveMessage(ev)
    conn.onerror   = (ev) => @_didFail(ev)
    conn.onclose   = (ev) => @_didClose(ev)

  disconnect: ->
    @_conn?.close()
    @_conn = undefined

  send: (data) ->
    @_conn?.send(data)

  _didOpen: (ev) ->
    console.log 'open', ev
    @trigger 'open', ev

  _didReceiveMessage: (ev) ->
    console.log 'message', ev
    @trigger 'message', ev.data

  _didFail: (ev) ->
    console.log 'fail', ev
    @trigger 'error', ev

  _didClose: (ev) ->
    console.log 'close', ev
    @trigger 'close', ev
