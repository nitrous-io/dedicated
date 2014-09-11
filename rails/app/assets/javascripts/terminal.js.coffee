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
      if @resizeTimeout
        window.clearTimeout(@resizeTimeout)
      @resizeTimeout = window.setTimeout(=>
        @resizeTimeout = null
        @term.sizeToFit()
      , 50)

    window.onfocus = =>
      return unless @term
      Terminal.focus = @term
      @term.redrawCursor()

    window.onblur = =>
      return unless @term
      Terminal.focus = null
      @term.redrawCursor()

  start: (url) ->
    @conn = conn = new WebSocket(url)
    conn.onopen    = (ev) => @open(ev)
    conn.onmessage = (ev) => @message(ev)
    conn.onerror   = (ev) => @error(ev)
    conn.onclose   = (ev) => @close(ev)

    term = @term

    term.on 'resize', (cols, rows)->
      conn.send "r|#{cols} #{rows}"

    term.handler = (data) ->
      conn.send "d|#{data}"

  stop: ->
    @term?.write('c|goodbye')
    @term?.removeAllListeners('resize')
    @term?.destroy()
    @conn?.close()

  focus: ->
    @term?.focus()

  open: (ev) ->
    @term.write '\x1b[r\x1b[H\x1b[J'

  message: (ev) ->
    if ev.data.slice(0, 2) == 'd|'
      @term.write ev.data.slice(2)

  error: (ev) ->

  close: (ev) ->

window.webFontLoaded = ->
  NitrousTerminal.term?.sizeToFit()
