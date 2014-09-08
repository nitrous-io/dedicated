#= require_self
#= require webfonts
#= require term

window.NitrousTerminal =
  term: null
  conn: null

  resizeTimeout: null

  init: ->
    @term = term = new Terminal(80, 25)
    term.open(document.body)
    term.sizeToFit()
    term.write 'Connecting...'

    window.onresize = =>
      return unless @term
      return if @resizeTimeout
      @resizeTimeout = window.setTimeout(=>
        @resizeTimeout = null
        @term.sizeToFit()
      , 602)

    window.onfocus = =>
      return unless @term
      Terminal.focus = @term
      @term.redrawCursor()

    window.onblur = =>
      return unless @term
      Terminal.focus = null
      @term.redrawCursor()

  start: (url, onResize) ->
    @conn = conn = new WebSocket(url)
    conn.onopen    = (ev) => @open(ev)
    conn.onmessage = (ev) => @message(ev)
    conn.onerror   = (ev) => @error(ev)
    conn.onclose   = (ev) => @close(ev)

    term = @term
    term.on 'resize', onResize
    term.handler = (data) ->
      conn.send data

  stop: ->
    @term?.removeAllListeners('resize')
    @term?.destroy()
    @conn?.close()

  focus: ->
    @term?.focus()

  open: (ev) ->
    @term.write '\u001b[r\u001b[H\u001b[J'

  message: (ev) ->
    @term.write(ev.data)

  error: (ev) ->

  close: (ev) ->

window.webFontLoaded = ->
  NitrousTerminal.term?.sizeToFit()
