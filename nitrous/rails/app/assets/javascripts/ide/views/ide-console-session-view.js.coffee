consoleSessionViewNextId = 0

App.IdeConsoleSessionView = Em.View.extend
  templateName: 'ide/console-session'
  consoleSessionViewId: null
  consoleSession: null
  classNames: ['ide-console-session-view']
  classNameBindings: ['consoleSession.active:active']

  iframeInjected: false

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

  injectIframe: ->
    return if @get 'iframeInjected'
    @set 'iframeInjected', true
    Em.run.scheduleOnce 'afterRender', this, ->
      controller = @get 'controller'
      consoleSession = @get 'consoleSession'
      $iframe = $('<iframe>')
      @$('.terminal-screen').append($iframe)
      iframeWindow = $iframe[0].contentWindow
      iframeDocument = iframeWindow.document

      # use pure js dom api because jquery fails to use iframe's window/document obj
      linkEl = iframeDocument.createElement('link')
      linkEl.setAttribute('rel', 'stylesheet')
      linkEl.setAttribute('href', location.origin + App.get('terminalCssPath'))
      iframeDocument.head.appendChild(linkEl)

      scriptEl = iframeDocument.createElement('script')
      scriptEl.setAttribute('src', location.origin + App.get('terminalJsPath'))
      scriptEl.onload = ->
        scriptEl.onload = null
        terminal = iframeWindow.NitrousTerminal
        terminal.init()
        terminal.term.write 'Connecting...'
        terminal.willOpen = =>
          controller.makeUrlAndConnect(terminal)
        terminal.didBlur = =>
          consoleSession.set 'focused', false
        # the following terminal.open() call is only to invoke willOpen()
        # it will be no-op since no URL is set
        # makeUrlConnect will then call terminal.open again after setting the URL.
        terminal.open()
      iframeDocument.body.appendChild(scriptEl)
