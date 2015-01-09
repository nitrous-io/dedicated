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
      newHeight = Math.max(($pane.offset().top + $pane.innerHeight()) - event.pageY, minHeight)

      $pane.css(height: "#{newHeight}px")
      @set 'height', newHeight

    $body.on "mouseup.#{id}", (event) =>
      @turnOffResizeEvents()

    $(window).on "mouseup.#{id}", (event) =>
      @turnOffResizeEvents()

  turnOffResizeEvents: ->
    id = @get 'elementId'
    $body = $('body')
    $body.off "mousemove.#{id}"
    $body.off "mouseup.#{id}"
    $(window).off "mouseup.#{id}"

  willDestroyElement: ->
    @_super()
    @turnOffResizeEvents()
