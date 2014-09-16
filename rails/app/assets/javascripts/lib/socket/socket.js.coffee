Lib.Socket = Em.Object.extend Em.Evented,
  url: null

  connect: ->
    @_conn = conn = new WebSocket(@get 'url') # not using @set to reduce overhead
    conn.onopen    = (ev) => @didOpen(ev)
    conn.onmessage = (ev) => @didReceiveMessage(ev)
    conn.onerror   = (ev) => @didFail(ev)
    conn.onclose   = (ev) => @didClose(ev)

  disconnect: ->
    @_conn?.close()
    @_conn = undefined

  send: (data) ->
    @_conn?.send(data)

  didOpen: (ev) ->
    console.log 'open', ev
    @trigger 'open', ev

  didReceiveMessage: (ev) ->
    console.log 'message', ev
    @trigger 'message', ev.data

  didFail: (ev) ->
    console.log 'fail', ev
    @trigger 'error', ev

  didClose: (ev) ->
    console.log 'close', ev
    @trigger 'close', ev
