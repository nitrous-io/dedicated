App.InjectTerminalIframeMixin = Em.Mixin.create
  terminal: null

  injectTerminalIframe: (callback) ->
    $iframe = $('<iframe>')
    @$('.terminal-screen').append($iframe)
    iframeWindow = $iframe[0].contentWindow
    iframeDocument = iframeWindow.document

    # hack to make iframe ready for injection in firefox
    iframeDocument.open()
    iframeDocument.close()

    # use pure js dom api because jquery fails to use iframe's window/document obj
    linkEl = iframeDocument.createElement('link')
    linkEl.setAttribute('rel', 'stylesheet')
    linkEl.setAttribute('href', location.origin + App.get('terminalCssPath'))
    iframeDocument.head.appendChild(linkEl)

    scriptEl = iframeDocument.createElement('script')
    scriptEl.setAttribute('src', location.origin + App.get('terminalJsPath'))

    scriptEl.onload = =>
      scriptEl.onload = null
      terminal = iframeWindow.NitrousTerminal
      terminal.init()
      terminal.focus()
      @set 'terminal', terminal
      callback?(terminal)

    iframeDocument.body.appendChild(scriptEl)
