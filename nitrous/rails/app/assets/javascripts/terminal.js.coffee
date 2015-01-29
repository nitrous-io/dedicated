#= require_self
#= require lib/socket/socket-connection
#= require webfonts
#= require term

window.NitrousTerminal =
  term: null
  conn: null
  url: null
  closed: false
  firstConnect: true
  afterOpen: null
  readonly: false

  resizeTimeout: null

  init: ->
    @conn = conn = new SocketConnection()
    conn.shouldRetry = false
    conn.onbeforeopen   = => @willOpen()
    conn.onopen    = (ev) => @didOpen(ev)
    conn.onmessage = (ev) => @didReceiveMessage(ev)
    conn.onerror   = (ev) => @didFail(ev)
    conn.onclose   = (ev) => @didClose(ev)

    @term = term = new Terminal(80, 25)
    term.open(document.body)
    term.sizeToFit()
    @attachHandlers()

    onresize = =>
      window.onresize = null
      return unless @term
      if @resizeTimeout
        window.clearTimeout(@resizeTimeout)
      @resizeTimeout = window.setTimeout(=>
        @resizeTimeout = null
        @term.sizeToFit()
      , 50)
      window.onresize = onresize
    onresize()

    window.onfocus = =>
      return unless @term
      Terminal.focus = @term
      @term.redrawCursor()

    window.onblur = =>
      return unless @term
      @didBlur()
      Terminal.focus = null
      @term.redrawCursor()

  attachHandlers: ->
    @term.on 'resize', (cols, rows) =>
      return if @conn.readyState != 1
      @conn.send "r|#{cols} #{rows}"

    @term.handler = (data) =>
      return if @readonly || @conn.readyState != 1
      @conn.send "d|#{data}"

  open: (nobeforeopen) ->
    @closed = false
    @conn.url = @url
    @conn.open(nobeforeopen)
    @conn.url = null

  retry: ->
    @conn.retry()

  destroy: ->
    @close()
    @term?.destroy()

  close: ->
    @conn?.close()
    @willOpen = null

  focus: ->
    @term?.focus()

  willOpen: ->
    # override me

  didBlur: ->
    # override me

  clear: ->
    @term.write '\x1b[r\x1b[H\x1b[J'

  didOpen: (ev) ->
    if @firstConnect
      @firstConnect = false
      @clear()
      @term.sizeToFit()
    else
      @term.write '\r\n'
    @afterOpen?()
    @afterOpen = null

  didReceiveMessage: (ev) ->
    type = ev.data.slice(0, 2)
    switch type
      when 'd|'
        @term.write ev.data.slice(2)
      when 'c|'
        @close()
      when 'h'
        @conn.send 'h'

  didFail: (ev) ->

  didClose: (ev) ->
    @closed = true
    @term?.removeAllListeners('resize')
    @term?.handler = ->
    @term?.write("\r\n[Session closed]")

window.webFontLoaded = ->
  NitrousTerminal.term?.sizeToFit()
