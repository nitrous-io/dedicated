#= require lib/encoding

class window.SocketConnection
  ws: null
  url: undefined
  protocols: undefined
  shouldRetry: true
  retries: 0
  retryTimeout: null

  constructor: (url, protocols) ->
    @url = url
    @protocols = protocols
    @retries = 0

  open: (nobeforeopen) ->
    if @ws
      @close()

    unless nobeforeopen
      @onbeforeopen()

    if !@url
      @ws = null
      return

    ws = @ws = if @protocols
      new WebSocket(@url, @protocols)
    else
      new WebSocket(@url)

    ws.binaryType = 'arraybuffer'
    ws.onopen    = (evt) => @onopen(evt)
    ws.onclose   = (evt) => @onclose(evt)
    ws.onerror   = (evt) => @onerror(evt)

    # Quick hack to make it backward compatible with
    # existing listeners that expect strings
    # New code that expects binary data can
    # retrieve it from evt.binaryData
    ws.onmessage = (evt) => 
      message = evt.data
      if message instanceof ArrayBuffer
        evt.binaryData = message
        evt.data = Lib.utf8Decode(message)
      @onmessage(evt)

    ws._onopen = =>
      @retries = 0

    ws._onclose = =>
      ws.removeEventListener 'open',  ws._onopen,  false
      ws.removeEventListener 'close', ws._onclose, false
      @retry() if @shouldRetry && !ws._noRetry

    ws.addEventListener 'open',  ws._onopen,  false
    ws.addEventListener 'close', ws._onclose, false

  retry: ->
    delay = if @retries <= 5
      2000
    else if @retries <= 10
      5000
    else if @retries > 10
      10000
    window.clearTimeout(@retryTimeout)
    @retryTimeout = window.setTimeout(=>
      @open()
    , delay)
    @retries += 1

  send: (data) ->
    if typeof(data) == 'string'
      data = Lib.utf8Encode(data)
    @ws.send(data)

  close: ->
    if ws = @ws
      ws._noRetry = true
      ws.close?.apply(ws, arguments)

  onbeforeopen: ->
  onopen: ->
  onclose: ->
  onmessage: ->
  onerror: ->

Object.defineProperties window.SocketConnection.prototype,
  binaryType     : { get: -> @ws?.binaryType }
  bufferedAmount : { get: -> @ws?.bufferedAmount }
  extensions     : { get: -> @ws?.extensions }
  protocol       : { get: -> @ws?.protocol }
  readyState     : { get: -> @ws?.readyState }
