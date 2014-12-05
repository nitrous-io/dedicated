App.IdeSidebarBorderView = Em.View.extend
  elementId: 'ide_sidebar_border_view'
  classNames: ['ide_side_pane_border_view']
  widthBinding: 'App.router.ideLayoutController.sidebarWidth'
  minPaneWidth: 240

  mouseDown: (event) ->
    event.preventDefault()

    $pane = $('#ide_sidebar_view')
    $body = $('body')

    minPaneWidth = @get 'minPaneWidth'
    id = @get 'elementId'

    $body.on "mousemove.#{id}", (event) =>
      newWidth = event.pageX - $pane.offset().left
      if newWidth < minPaneWidth
        newWidth = minPaneWidth

      $pane.css 'width', newWidth
      @set 'width', newWidth

    $body.on "mouseup.#{id}", (event) =>
      this.turnOffResizeEvents()

  turnOffResizeEvents: ->
    id = @get 'elementId'
    $body = $('body')
    $body.off "mousemove.#{id}"
    $body.off "mouseup.#{id}"

  willDestroyElement: ->
    @_super()
    this.turnOffResizeEvents()
