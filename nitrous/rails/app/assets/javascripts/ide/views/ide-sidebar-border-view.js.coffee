App.IdeSidebarBorderView = Em.View.extend
  elementId: 'ide-sidebar-border-view'
  classNames: ['ide-side-pane-border']
  width: 240
  minWidth: 240

  mouseDown: (event) ->
    event.preventDefault()

    $pane = $('#ide-sidebar-view')
    $body = $('body')

    minWidth = @get 'minWidth'
    id = @get 'elementId'

    $body.on "mousemove.#{id}", (event) =>
      newWidth = Math.max(event.pageX - $pane.offset().left, minWidth)

      $pane.css(width: "#{newWidth}px")
      @set 'width', newWidth

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
