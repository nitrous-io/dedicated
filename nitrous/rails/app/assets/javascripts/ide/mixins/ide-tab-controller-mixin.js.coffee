App.IdeTabControllerMixin = Em.Mixin.create
  modelClass: Em.required()

  tabCount: (->
    @get('content').length
  ).property 'content.length'

  activeTab: (->
    content = @get 'content'
    content.findProperty 'active', true
  ).property 'content.@each.active'

  activeTabIndex: (->
    content = @get 'content'
    activeTab = @get 'activeTab'
    content.indexOf activeTab
  ).property 'activeTab'

  hasNoTab: (->
    content = @get 'content'
    !content || content.length == 0
  ).property 'content', 'content.length'

  nextTab: (->
    activeTabIndex = @get 'activeTabIndex'
    tabCount = @get 'tabCount'
    index = (activeTabIndex + 1) % tabCount
    @get "content.#{index}"
  ).property 'activeTabIndex', 'tabCount'

  previousTab: (->
    activeTabIndex = @get 'activeTabIndex'
    tabCount = @get 'tabCount'
    index = (((activeTabIndex - 1) % tabCount) + tabCount) % tabCount
    @get "content.#{index}"
  ).property 'activeTabIndex', 'tabCount'

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
        Em.run.next ->
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
    if @get('tabCount') > 1
      nextTab = @get 'nextTab'
      this.makeTabActive nextTab

  makePreviousTabActive: ->
    if @get('tabCount') > 1
      previousTab = @get 'previousTab'
      this.makeTabActive previousTab

  focusActiveTab: ->
    if activeTab = @get 'activeTab'
      activeTab.set 'focused', true
