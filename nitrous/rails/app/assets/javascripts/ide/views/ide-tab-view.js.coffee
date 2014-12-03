App.IdeTabView = Em.View.extend App.IdeTabViewMixin, App.IdeDragDropViewMixin,
  templateName: 'templates/box/ide/tab'
  classNames: ['ide_tab_view']
  classNameBindings: ['content.active']
  attributeBindings: 'draggable'
  tagName: 'li'
  content: null

  closeTab: (->
    if @get 'content.path.deleted'
      this.remove()
  ).observes 'content.path.deleted'

  dragStart: (event) ->
    @_super event
    @set 'controller.draggedTab', @get('content')

  dragEnter: (event) ->
    @_super event
    tabs = @get 'controller.content'
    currentTab = @get 'content'
    if tabs && currentTab
      @set 'controller.droppableIndex', tabs.indexOf(currentTab)

  dragEnd: (event) ->
    @_super event
    tabs = @get 'controller.content'
    draggedTab = @get 'controller.draggedTab'
    droppableIndex = @get 'controller.droppableIndex'
    if tabs && draggedTab && droppableIndex
      tabs.removeObject draggedTab
      tabs.insertAt droppableIndex, draggedTab
