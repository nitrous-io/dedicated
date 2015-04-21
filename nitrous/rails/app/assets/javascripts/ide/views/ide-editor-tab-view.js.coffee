App.IdeEditorTabView = klass = Em.View.extend App.IdeTabViewMixin, App.IdeDragDropViewMixin,
  templateName: 'ide/editor-tab'
  classNames: ['ide-editor-tab-view']
  classNameBindings: ['content.active:selected', 'dropLeft:drop-left', 'dropRight:drop-right']
  attributeBindings: 'draggable'
  tagName: 'li'
  content: null

  dragEnters: 0
  dropLeft: false
  dropRight: false

  pathDidGetDeleted: (->
    if @get 'content.path.deleted'
      this.remove()
  ).observes 'content.path.deleted'

  dragStart: (event) ->
    @_super.apply this, arguments
    tab = @get 'content'
    klass.draggedTab = tab
    klass.draggedTabEditorPane = @get('parentView.editorPane')

  dragEnter: (event) ->
    @_super.apply this, arguments
    @incrementProperty 'dragEnters'
    currentPane = @get('parentView.editorPane')
    currentTab = @get 'content'
    return unless currentPane && currentTab
    currentTabIndex = currentPane.indexOf(currentTab)
    $tab = $(event.target).closest('.ide-editor-tab-view')

    if $tab.length > 0
      tabLeft = $tab.offset().left
      tabWidth = $tab.width()
      x = event.originalEvent.x

      # if current mouse position is over the first half of button,
      # drop to the left, else drop to the right
      klass.dropEditorPane = currentPane
      klass.dropIndex = if x < (tabLeft + tabWidth / 2)
        @set 'dropLeft', true
        @set 'dropRight', false
        currentTabIndex
      else
        @set 'dropLeft', false
        @set 'dropRight', true
        currentTabIndex + 1

  dragLeave: (event) ->
    @_super.apply this, arguments
    @decrementProperty 'dragEnters'
    if @get('dragEnters') == 0
      @setProperties
        dropLeft: false
        dropRight: false

  dragEnd: (event) ->
    @_super.apply this, arguments
    tab = klass.draggedTab
    # TODO: fix drag-dropping of tabs with unsaved changes
    return if !tab.get('clean')
    fromPane = klass.draggedTabEditorPane
    toPane = klass.dropEditorPane
    controller = @get 'controller'

    dropIndex = klass.dropIndex
    if controller && tab && fromPane && toPane && dropIndex?
      dropIndex = Math.min(toPane.get('length'), dropIndex)

      if fromPane == toPane
        # removal of original tab affects indices
        originalIndex = fromPane.indexOf(tab)
        if originalIndex < dropIndex
          dropIndex -= 1

      fromPane.makeActive()
      controller.removeTab(tab)
      tab.set 'active', false
      toPane.insertAt(dropIndex, tab)
      toPane.makeActive()
      controller.makeTabActive(tab)

    klass.draggedTab = null
    klass.draggedTabEditorPane = null

    @setProperties
      dragEnters: 0
      dropLeft: false
      dropRight: false

  drop: (event) ->
    @_super.apply this, arguments
    @setProperties
      dragEnters: 0
      dropLeft: false
      dropRight: false

  actions:
    didClickTab: (tab) ->
      @get('parentView.editorPane')?.makeActive()
      @_super.apply this, arguments

    didClickClose: (tab) ->
      @get('parentView.editorPane')?.makeActive()
      @_super.apply this, arguments

klass.reopenClass
  draggedTab: null # tab being dragged
  draggedTabEditorPane: null # original editor pane that the tab being dragged belongs to
  dropEditorPane: null
  dropIndex: null # index where the currently dragged tab would be inserted
