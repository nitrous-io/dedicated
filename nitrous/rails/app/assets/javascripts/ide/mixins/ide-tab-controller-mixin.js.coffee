App.IdeTabControllerMixin = Em.Mixin.create
  modelClass: Em.required()

  activeTab: (->
    if content = @get 'content'
      content.findProperty 'active', true
  ).property 'content.@each.active'

  activeTabIndex: (->
    content = @get 'content'
    activeTab = @get 'activeTab'
    if content && activeTab
      content.indexOf activeTab
  ).property 'activeTab'

  hasNoTab: (->
    content = @get 'content'
    !content || content.get('length') == 0
  ).property 'content.length'

  # return false if active tab is a text editor instance
  # return true otherwise (there are no tabs or active tab is e.g. an image editor)
  # a negative because it's used as a binding to disable menu items
  isActiveTabNotAnEditor: (->
    contentType = @get 'activeTab.contentType'
    !contentType || contentType != "text"
  ).property 'activeTab.contentType'

  nextTab: (->
    activeTabIndex = @get 'activeTabIndex'
    tabCount = @get 'content.length'
    index = (activeTabIndex + 1) % tabCount
    @objectAt(index)
  ).property 'activeTabIndex', 'content.length'

  previousTab: (->
    activeTabIndex = @get 'activeTabIndex'
    tabCount = @get 'content.length'
    index = (((activeTabIndex - 1) % tabCount) + tabCount) % tabCount
    @objectAt(index)
  ).property 'activeTabIndex', 'content.length'

  addTab: (tab) ->
    if content = @get('content')
      content.pushObject tab
      this.makeTabActive tab

  createTab: (attrs={}, modelClass=null) ->
    modelClass ||= @get('modelClass')
    tab = modelClass.create attrs
    this.addTab tab
    tab

  makeTabActive: (tab) ->
    unless tab.get 'active'
      content = @get 'content'
      content.forEach (t) ->
        t.set 'active', t == tab
        t.set 'focused', t == tab

  removeTab: (tab) ->
    tab = @get 'activeTab' unless tab
    content = @get 'content'
    if tab.get 'active'
      if content.get('lastObject') == tab
        this.makePreviousTabActive()
      else
        this.makeNextTabActive()
    content.removeObject tab

  makeNextTabActive: ->
    if @get('content.length') > 1
      nextTab = @get 'nextTab'
      this.makeTabActive nextTab

  makePreviousTabActive: ->
    if @get('content.length') > 1
      previousTab = @get 'previousTab'
      this.makeTabActive previousTab

  focusActiveTab: ->
    if activeTab = @get 'activeTab'
      activeTab.set 'focused', true
