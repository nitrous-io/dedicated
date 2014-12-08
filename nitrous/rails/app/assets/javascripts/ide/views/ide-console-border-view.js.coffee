App.IdeConsoleBorderView = Em.View.extend
  elementId: 'ide_console_border_view'
  minConsoleHeight: null
  heightBinding: 'App.router.ideLayoutController.consoleHeight'

  mouseDown: (event)->
    event.preventDefault()

    $consoleView = $('#ide_console_view')
    $body = $('body')
    $ideBody = $('#ide_body')

    minConsoleHeight = 180

    doResize = (pageY) =>
      maxConsoleHeight = $ideBody.innerHeight() - 4 # leave some room to resize back
      newHeight = ( $consoleView.offset().top + $consoleView.innerHeight() ) - pageY

      if newHeight < minConsoleHeight
        newHeight = minConsoleHeight

      if newHeight > maxConsoleHeight
        newHeight = maxConsoleHeight

      $consoleView.css 'height', newHeight
      @set 'height', newHeight

    $body.on
      'mousemove.ideConsoleResize': (event, frameY) ->
        # for the bubbled up events we have to calcultate the pageY
        doResize (event.pageY || $consoleView.offset().top + frameY)

      'mouseup.ideConsoleResize': (event) =>
        this.turnOffResizeEvents()

    # bubble-up the events to main body from iframe
    $('#ide_console_view iframe').contents().find('body').on
      'mousemove.ideConsoleResize': (event) ->
        $body.trigger 'mousemove.ideConsoleResize', [ event.pageY ]

      'mouseup.ideConsoleResize': (event) ->
        $body.trigger 'mouseup.ideConsoleResize'

  turnOffResizeEvents: ->
    $('body').off 'mousemove.ideConsoleResize'
    $('body').off 'mouseup.ideConsoleResize'
    $('#ide_console_view iframe').contents().find('body').off 'mouseup.ideConsoleResize'
    $('#ide_console_view iframe').contents().find('body').off 'mousemove.ideConsoleResize'

  willDestroyElement: ->
    @_super()
    this.turnOffResizeEvents()
