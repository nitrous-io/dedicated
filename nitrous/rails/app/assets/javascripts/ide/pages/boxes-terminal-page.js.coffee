#= require jquery
#= require underscore
#= require term
#= require ../lib/socket-connection
#= require ../lib/socket
#= require ../lib/auth-token

parentWindow = window.parent
parentWindow = null if parentWindow == window

# prevent esc from disconnecting websocket connection in firefox
$(window).keydown (e) ->
  e.preventDefault() if e.keyCode == 27

window.App ||= {}

class App.BoxesTerminalPage
  constructor: (serverURI, boxId) ->
    self = this

    @serverURI = serverURI
    @boxId = boxId

    @term = new Terminal(80, 6)
    @term.open()
    @term.sizeToFit()

    blurTimeout = null
    $(window).blur (e) =>
      # stupid hack to prevent blur event from screwing with iframe on ipads
      blurTimeout = window.setTimeout(=>
        if Terminal.focus != null
          Terminal.focus = null
          @term.redrawCursor()
        if parentWindow
          consoleSession = parentWindow.App.get 'router.ideConsoleController.activeTab'
          if consoleSession.get 'focused'
            consoleSession.set 'focused', false
      , 100)

    $(window).blur() unless document.hasFocus?()

    @term.originalFocus = @term.focus
    @term.focus = ->
      window.clearTimeout blurTimeout
      this.originalFocus()
      if parentWindow
        consoleSession = parentWindow.App.get 'router.ideConsoleController.activeTab'
        if !consoleSession.get 'focused'
          consoleSession.set 'focused', true

    $(window).focus =>
      @term.focus()

    $(window).on 'touchstart', (e) =>
      @term.focus()

    $(window).on 'touchend', =>
      @term.focus()

    @term.originalKeyDown = @term.keyDown
    @term.keyDown = (event) ->
      switch
        when self.matchesBlurKeybinding event
          self.blurConsole event
        when self.matchesNextTabKeybinding event
          self.makeNextTabActive event
        when self.matchesPreviousTabKeybinding event
          self.makePreviousTabActive event
        when self.matchesFullConsoleScreenKeybinding event
          self.makeConsoleFullScreen event
        when self.matchesNewConsoleScreenKeybinding event
          self.createTab event
        when self.matchesCloseConsoleScreenKeybinding event
          self.removeTab event
        when self.matchesRefreshTreeKeybinding event
          self.refreshTree event
        when self.matchesMacCopyKeybinding event
          null
        when self.matchesFileFinderKeybinding event
          self.fileFinder event
        else
          this.originalKeyDown event

    window.onresize = =>
      @term.sizeToFit()

    @term.handler = (data) =>
      if @started
        @socket.send 'd', data

    @term.on 'resize', (cols, rows) =>
      if @started
        @socket.send 'r', cols, rows

    @term.element.focus()

    this.connect()

  connect: ->
    @connection = connection = new App.SocketConnection "#{@serverURI}/ws", 'rainbow'
    @socket = socket = new App.Socket 'ssh', connection
    @started = false

    @socket.on 'p', (data) =>
      @term.write data

    connection.on 'authenticated', _.bind(@_didAuthenticate, this)
    connection.on 'close', _.bind(@_didClose, this)
    connection.connect()

  _didAuthenticate: ->
    @socket.send 'init', =>
      @socket.send 'start', @boxId, (err) =>
        return if err
        @reconnecting = false
        @started = true
        @term.sizeToFit()

  _didClose: ->
    unless @reconnecting
      @term.write "\r\nConnection lost. Reconnecting...\r\n"
      @reconnecting = true

  focus: ->
    window.focus()
    @term.focus()

  # keybinding is shift+ctrl+c / ctrl+alt+c
  matchesBlurKeybinding: (event) ->
    (Terminal.isMac && event.shiftKey && event.ctrlKey && event.keyCode == 67) ||
    (!Terminal.isMac && event.ctrlKey && event.altKey && event.keyCode == 67)

  # keybinding is shift+ctrl+]
  matchesNextTabKeybinding: (event) ->
    event.shiftKey && event.ctrlKey && event.keyCode == 221

  # keybinding is shift+ctrl+[
  matchesPreviousTabKeybinding: (event) ->
    event.shiftKey && event.ctrlKey && event.keyCode == 219

  # keybinding is shift+ctrl+f / ctrl+alt+f
  matchesFullConsoleScreenKeybinding: (event) ->
    (Terminal.isMac && event.shiftKey && event.ctrlKey && event.keyCode == 70) ||
    (!Terminal.isMac && event.ctrlKey && event.altKey && event.keyCode == 70)

  # keybinding is shift+ctrl+n / ctrl+alt+n
  matchesNewConsoleScreenKeybinding: (event) ->
    (Terminal.isMac && event.shiftKey && event.ctrlKey && event.keyCode == 78) ||
    (!Terminal.isMac && event.ctrlKey && event.altKey && event.keyCode == 78)

  # keybinding is shift+ctrl+w / ctrl+alt+w
  matchesCloseConsoleScreenKeybinding: (event) ->
    (Terminal.isMac && event.shiftKey && event.ctrlKey && event.keyCode == 87) ||
    (!Terminal.isMac && event.ctrlKey && event.altKey && event.keyCode == 87)

  # keybinding is shift+ctrl+r / ctrl+alt+r
  matchesRefreshTreeKeybinding: (event) ->
    (Terminal.isMac && event.shiftKey && event.ctrlKey && event.keyCode == 82) ||
    (!Terminal.isMac && event.ctrlKey && event.altKey && event.keyCode == 82)

  # keybinding is cmd+c
  matchesMacCopyKeybinding: (event) ->
    Terminal.isMac && event.metaKey && event.keyCode == 67

  # keybinding is shift+ctrl+p
  matchesFileFinderKeybinding: (event) ->
    event.shiftKey && event.ctrlKey && event.keyCode == 80

  blurConsole: (event) ->
    if parentWindow
      consoleSession = parentWindow.App.get 'router.ideConsoleController.activeTab'
      if consoleSession.get 'focused'
        consoleSession.set 'focused', false
      if document = parentWindow.App.get 'router.ideEditorController.activeTab'
        document.set 'focused', true

      parentWindow.App.util.mixpanelIncrement 'ide_blurred_console_count'

    Terminal.cancel event
    false

  makeNextTabActive: (event) ->
    if parentWindow
      ideConsoleController = parentWindow.App.get 'router.ideConsoleController'
      ideConsoleController.makeNextTabActive()
    Terminal.cancel event
    false

  makePreviousTabActive: (event) ->
    if parentWindow
      ideConsoleController = parentWindow.App.get 'router.ideConsoleController'
      ideConsoleController.makePreviousTabActive()
    Terminal.cancel event
    false

  makeConsoleFullScreen: (event) ->
    if parentWindow
      ideConsoleController = parentWindow.App.get 'router.ideConsoleController'
      ideConsoleController.toggleProperty 'fullscreen'
      parentWindow.App.util.mixpanelIncrement 'ide_toggled_console_fullscreen_count'
    Terminal.cancel event
    false

  createTab: (event) ->
    if parentWindow
      ideConsoleController = parentWindow.App.get 'router.ideConsoleController'
      ideConsoleController.createTab()
    Terminal.cancel event
    false

  removeTab: (event) ->
    if parentWindow
      ideConsoleController = parentWindow.App.get 'router.ideConsoleController'
      consoleSession = ideConsoleController.get 'activeTab'
      ideConsoleController.removeTab consoleSession
    Terminal.cancel event
    false

  refreshTree: (event) ->
    if parentWindow
      ideSidebarController = parentWindow.App.get 'router.ideSidebarController'
      if ideSidebarController.get 'sftpSocketReady'
        ideSidebarController.refreshAll()
    Terminal.cancel event
    false

  fileFinder: (event) ->
    if parentWindow
      ideConsoleController = parentWindow.App.get 'router.ideConsoleController'
      if !ideConsoleController.get('fullscreen')
        ideConsoleController.get('controllers.ideFileFinder').showFindDialog()

    Terminal.cancel event
    false
