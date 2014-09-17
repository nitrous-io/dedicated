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
    @trigger 'open', ev

  didReceiveMessage: (ev) ->
    @trigger 'message', ev.data

  didFail: (ev) ->
    @trigger 'error', ev

  didClose: (ev) ->
    @trigger 'close', ev
