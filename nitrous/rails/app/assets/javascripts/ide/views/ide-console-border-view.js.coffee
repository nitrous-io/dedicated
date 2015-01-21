App.IdeConsoleBorderView = Em.View.extend
  elementId: 'ide-console-border-view'
  classNames: ['ide-side-pane-border', 'horizontal']
  minHeight: 165
  height: 165

  mouseDown: (event) ->
    event.preventDefault()

    $pane = $('#ide-console-view')
    $body = $('body')

    minHeight = @get 'minHeight'
    id = @get 'elementId'

    $body.on "mousemove.#{id}", (event) =>
      newHeight = Math.floor(Math.max(($pane.offset().top + $pane.innerHeight()) - event.pageY, minHeight))

      $pane.css(height: "#{newHeight}px")
      @set 'height', newHeight

    $body.on "mouseup.#{id}", (event) =>
      @turnOffResizeEvents()

    $(window).on "mouseup.#{id}", (event) =>
      @turnOffResizeEvents()

    # place a transparent layer on top of terminal screen
    # to prevent it from blocking mouse events
    $('<div class="iframe-cover" style="position: absolute; width: 100%; height: 100%; top: 0; left: 0; background: transparent">').appendTo($pane.find('.console-sessions'))

  turnOffResizeEvents: ->
    id = @get 'elementId'
    $body = $('body')
    $body.off "mousemove.#{id}"
    $body.off "mouseup.#{id}"
    $(window).off "mouseup.#{id}"
    $('#ide-console-view .iframe-cover').remove()

  willDestroyElement: ->
    @_super()
    @turnOffResizeEvents()
