#= require ./ot

class ot.CollabSocketAdapter
  constructor: (sock, fileName) ->
    @sock = sock
    @fileName = fileName

    sock.on "registered|#{fileName}", (data) =>
      # noop

    sock.on "left|#{fileName}", (data) =>
      clientId = data.conn_id
      @trigger 'client_left', clientId

    sock.on "disconnected|#{fileName}", (data) =>
      clientId = data.conn_id
      @trigger 'client_left', clientId

    sock.on "joined|#{fileName}", (data) =>
      clientId = data.conn_id
      name = data.username
      @trigger 'set_name', clientId, name

    sock.on "ok|#{fileName}", =>
      @trigger 'ack'

    sock.on "op|#{fileName}", (data) =>
      clientId = data[0]
      operation = data[1]
      selection = data[2]
      @trigger 'operation', operation
      @trigger 'selection', clientId, selection

    sock.on "sel|#{fileName}", (data) =>
      clientId = data[0]
      selection = data[1]
      @trigger 'selection', clientId, selection

    @onReconnect = =>
      @trigger 'reconnect'

    sock.on 'reconnect', @onReconnect

  sendOperation: (revision, operation, selection) ->
    @sock.send @fileName, 'op', revision, operation, selection

  sendSelection: (selection) ->
    @sock.send @fileName, 'sel', selection

  registerCallbacks: (cb) ->
    @callbacks = cb

  trigger: (event) ->
    args = Array.prototype.slice.call(arguments, 1)
    action = @callbacks && @callbacks[event]
    action.apply(this, args) if action

  off: ->
    sock = @sock
    fileName = @fileName

    sock.off "registered|#{fileName}", this
    sock.off "left|#{fileName}", this
    sock.off "joined|#{fileName}", this
    sock.off "ok|#{fileName}", this
    sock.off "op|#{fileName}", this
    sock.off "sel|#{fileName}", this

    sock.off "reconnect", @onReconnect
