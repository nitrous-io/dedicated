App.ContainersNewView = Em.View.extend
  elementId: 'containers-new-view'
  classNames: ['ui', 'card']

  showAdvanced: false

  actions:
    toggleShowAdvanced: ->
      @$('#toggle-advanced-btn').blur()
      @toggleProperty 'showAdvanced'
      false
