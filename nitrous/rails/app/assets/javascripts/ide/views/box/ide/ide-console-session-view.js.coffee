consoleSessionViewNextId = 0

App.IdeConsoleSessionView = Em.View.extend
  templateName: 'templates/box/ide/console-session'
  consoleSessionViewId: null
  consoleSession: null
  classNameBindings: ['consoleSession.active:active']

  init: ->
    @set 'consoleSessionViewId', ++consoleSessionViewNextId
    @_super.apply this, arguments

  didInsertElement: ->
    @_super()
    $(window).bind "orientationchange.ideConsoleSessionView#{@get 'consoleSessionViewId'}", _.bind(@didChangeOrientation, this)

  willDestroyElement: ->
    @_super()
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
