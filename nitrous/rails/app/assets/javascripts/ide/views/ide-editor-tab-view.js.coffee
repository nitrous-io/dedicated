App.IdeEditorTabView = Em.View.extend App.IdeTabViewMixin, App.IdeDragDropViewMixin,
  templateName: 'ide/editor-tab'
  classNames: ['ide-editor-tab-view']
  classNameBindings: ['content.active:selected']
  attributeBindings: 'draggable'
  tagName: 'li'
  content: null

  pathDidGetDeleted: (->
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

  actions:
    didClickTab: ->
      @get('parentView.editorPane')?.makeActive()
      @_super.apply this, arguments

    didClickClose: ->
      @get('parentView.editorPane')?.makeActive()
      @_super.apply this, arguments
