#= require polyfills
#= require encoding
#= require_self
#= require lib/socket/socket-connection
#= require term

parentWindow = window.parent
parentWindow = null if parentWindow == window

isMac = navigator.userAgent.indexOf("Macintosh") != -1

utf8Decoder = new window.TextDecoder('utf8')

utf8Decode = (data) ->
  utf8Decoder.decode(new DataView(data))

appendBuffer = (buffer1, buffer2) ->
  if !buffer1? || buffer1.byteLength == 0
    return buffer2
  tmp = new Uint8Array(buffer1.byteLength + buffer2.byteLength)
  tmp.set(new Uint8Array(buffer1), 0)
  tmp.set(new Uint8Array(buffer2), buffer1.byteLength)
  tmp.buffer

window.NitrousTerminal =
  term: null
  conn: null
  url: null
  closed: false
  firstConnect: true
  readonly: false
  binaryMode: false
  ide: false

  resizeTimeout: null

  dataBuffer: null
  bufferTimeout: null

  init: ->
    @conn = conn = new SocketConnection()
    conn.onbeforeopen   = => @willOpen()
    conn.onopen    = (ev) => @didOpen(ev)
    conn.onmessage = (ev) => @didReceiveMessage(ev)
    conn.onerror   = (ev) => @didFail(ev)
    conn.onclose   = (ev) => @didClose(ev)

    @term = term = new Terminal(80, 25)
    term.open(document.body)
    term.sizeToFit()

    term.originalKeyDown = @term.keyDown
    term.keyDown = (event) =>
      if @ide
        switch
          when @matchesSwitchConsoleEditorKeybinding(event)
            return @handleSwitchConsoleEditorEvent(event)
          when @matchesFullConsoleScreenKeybinding(event)
            return @handleFullConsoleScreenEvent(event)
      term.originalKeyDown(event)

    term.on 'resize', (cols, rows) =>
      return if @conn.readyState != 1
      @conn.send "r|#{cols} #{rows}"

    term.handler = (data) =>
      return if @readonly || @closed || @conn.readyState != 1
      @conn.send "d|#{data}"

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

    document.body.addEventListener('copy', @handleCopy, false)

  open: (nobeforeopen) ->
    @conn.binaryMode = @binaryMode

    @conn.url = @url
    @conn.open(nobeforeopen)
    @conn.url = null

  retry: ->
    @conn.retry()

  destroy: ->
    @close()
    @term?.destroy()

  close: (retry=false) ->
    @conn?.close(retry)
    if !retry
      @willOpen = null

  focus: ->
    @term?.focus()

  willOpen: ->
    # override me

  didBlur: ->
    # override me

  clear: ->
    @term.write '\x1b[r\x1b[H\x1b[J'

  handleCopy: ->
    sel = window.getSelection?()
    range = sel.getRangeAt?(0)
    text = sel.toString?()
    tempEl = document.createElement('pre')
    # return if browser required features are missing
    return true unless sel && sel.selectAllChildren && sel.removeAllRanges && sel.addRange && range && text && tempEl.textContent?
    # make temp element invisible
    tempEl.style.whiteSpace = 'pre'
    tempEl.style.position = 'fixed'
    tempEl.style.left = '100%'
    tempEl.style.opacity = '0'
    # strip trailing whitespaces
    tempEl.textContent = text.replace(/\s+$/gm, '')
    document.body.appendChild(tempEl)
    sel.selectAllChildren(tempEl)
    window.setTimeout(->
      tempEl.parentNode.removeChild(tempEl)
      sel.removeAllRanges()
      sel.addRange(range)
    , 10)

  handleData: (data) ->
    window.clearTimeout(@bufferTimeout)
    @dataBuffer = appendBuffer(@dataBuffer, data)
    decoded = utf8Decode(@dataBuffer)

    flush = =>
      @dataBuffer = new ArrayBuffer(0)
      @term.write(decoded)

    # if invalid utf8 char is detected, buffer with timeout
    if decoded.length > 0 && decoded[decoded.length - 1] == "\uFFFD"
      @bufferTimeout = window.setTimeout(=>
        flush()
        @bufferTimeout = null
      , 200)
      return

    flush()

  didOpen: (ev) ->
    @closed = false
    @term.sizeToFit()
    if @firstConnect
      @firstConnect = false
      @clear()
    else
      @term.write '\r\n'

  didReceiveMessage: (ev) ->
    type = ev.data.slice(0, 2)
    switch type
      when 'd|'
        if @binaryMode
          @handleData(ev.binaryData.slice(2))
        else
          @term.write(ev.data.slice(2))
      when 'c|'
        @close()
      when 'h'
        @conn.send 'h'

  didFail: (ev) ->

  didClose: (ev) ->
    @closed = true
    @term?.write("\r\n[Session closed]")

  handleSwitchConsoleEditorEvent: (ev) ->
    if parentWindow
      Terminal.cancel ev
      parentWindow.App.get('router.ideMainMenuController').viewFocusEditor()
      return false
    true

  handleFullConsoleScreenEvent: (ev) ->
    if parentWindow
      Terminal.cancel ev
      parentWindow.App.get('router.ideMainMenuController').viewToggleFullScreenConsole()
      return false
    true

  # keybinding is shift+ctrl+c / ctrl+alt+c
  matchesSwitchConsoleEditorKeybinding: (event) ->
    (isMac && event.shiftKey && event.ctrlKey && event.keyCode == 67) ||
    (!isMac && event.ctrlKey && event.altKey && event.keyCode == 67)

  # keybinding is shift+ctrl+f / ctrl+alt+f
  matchesFullConsoleScreenKeybinding: (event) ->
    (isMac && event.shiftKey && event.ctrlKey && event.keyCode == 70) ||
    (!isMac && event.ctrlKey && event.altKey && event.keyCode == 70)

window.webFontLoaded = ->
  NitrousTerminal.term?.sizeToFit()
