# TODO: make vertical border view or something and make it inherit from it
App.IdeCollaborationPaneBorderView = Em.View.extend
  elementId: 'ide_collaboration_pane_border_view'
  classNames: ['ide_side_pane_border_view']
  widthBinding: 'App.router.ideLayoutController.collaborationPaneWidth'
  minPaneWidth: 240

  mouseDown: (event) ->
    event.preventDefault()

    $pane = $('#ide_collaboration_pane_view')
    $body = $('body')

    minPaneWidth = @get 'minPaneWidth'
    id = @get 'elementId'

    $body.on "mousemove.#{id}", (event) =>
      newWidth = $body.width() - event.pageX
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
