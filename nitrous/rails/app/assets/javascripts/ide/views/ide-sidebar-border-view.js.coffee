App.IdeSidebarBorderView = Em.View.extend
  elementId: 'ide-sidebar-border-view'
  classNames: ['ide-side-pane-border']
  width: 250
  minWidth: 250

  mouseDown: (event) ->
    event.preventDefault()

    $pane = $('#ide-sidebar-view')
    $body = $('body')

    minWidth = @get 'minWidth'
    id = @get 'elementId'

    $body.on "mousemove.#{id}", (event) =>
      newWidth = Math.max(event.pageX - $pane.offset().left, minWidth)
      @setPaneWidth(newWidth)

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

  setPaneWidth: (width) ->
    $pane = $('#ide-sidebar-view')
    $pane.css(width: "#{width}px")
    $pane.find('.tree').css(width: "#{width}px")
    @set 'width', width

  didInsertElement: ->
    Em.run.scheduleOnce 'afterRender', this, ->
      @setPaneWidth(@get('width'))

  willDestroyElement: ->
    @_super()
    @turnOffResizeEvents()
