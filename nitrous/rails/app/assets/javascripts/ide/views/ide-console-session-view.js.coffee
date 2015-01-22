#= require ../../mixins/inject-terminal-iframe-mixin

consoleSessionViewNextId = 0

App.IdeConsoleSessionView = Em.View.extend App.InjectTerminalIframeMixin,
  templateName: 'ide/console-session'
  consoleSessionViewId: null
  consoleSession: null
  classNames: ['ide-console-session-view']
  classNameBindings: ['consoleSession.active:active']

  iframeInjected: false

  opsSocketReadyBinding: Em.Binding.oneWay 'App.router.ideController.opsSocket.started'

  opsSocketChanged: (->
    if @get('opsSocketReady') && @get('terminal') && @get('terminal.closed')
      @reattachTerminal()
  ).observes 'opsSocketReady'

  init: ->
    @set 'consoleSessionViewId', ++consoleSessionViewNextId
    @_super.apply this, arguments

  didInsertElement: ->
    @_super()
    $(window).bind "orientationchange.ideConsoleSessionView#{@get 'consoleSessionViewId'}", _.bind(@didChangeOrientation, this)
    @injectIframe()

  willDestroyElement: ->
    @_super()
    @$('iframe')[0]?.contentWindow?.NitrousTerminal?.destroy()
    $(window).unbind "orientationchange.ideConsoleSessionView#{@get 'consoleSessionViewId'}"

  focusDidChange: (->
    if focused = @get 'consoleSession.focused'
      if iframeWindow = @$('iframe')[0]?.contentWindow
        if activeDocument = App.get 'router.ideEditorController.activeTab'
          activeDocument.set 'focused', false
        iframeWindow.focus()
        iframeWindow.App?.page?.focus()
  ).observes 'consoleSession.focused'

  didChangeOrientation: ->
    window.setTimeout(=>
      if iframe = @$('iframe')[0]
        iframe.style.width = $('body').width() + 'px';
        iframe.contentWindow?.App?.page?.term?.sizeToFit?()
        window.setTimeout(->
          iframe.style.width = '100%'
        , 10)
    , 800)

  reattachTerminal: ->
    terminal = @get 'terminal'
    controller = @get 'controller'
    terminal.willOpen = =>
      controller.makeUrlAndConnect(terminal)
    terminal.afterOpen = =>
      terminal.attachHandlers()
    terminal.open()

  injectIframe: ->
    return if @get 'iframeInjected'
    @set 'iframeInjected', true
    Em.run.scheduleOnce 'afterRender', this, ->
      controller = @get 'controller'
      consoleSession = @get 'consoleSession'

      @injectTerminalIframe (terminal) =>
        terminal.term.write 'Connecting...'
        terminal.willOpen = =>
          controller.makeUrlAndConnect(terminal)
        terminal.didBlur = =>
          consoleSession.set 'focused', false
        # the following terminal.open() call is only to invoke willOpen()
        # it will be no-op since no URL is set
        # makeUrlConnect will then call terminal.open again after setting the URL.
        terminal.open()
