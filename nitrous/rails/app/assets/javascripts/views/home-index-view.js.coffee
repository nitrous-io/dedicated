App.HomeIndexView = Em.View.extend
  elementId: 'home-index-view'

  handleNoHost: (->
    if App.get 'noHost'
      @get('controller').send 'handleNoHost'
  ).observes 'App.noHost'

  redirectToFirstHost: (->
    if App.get('hosts.loaded') && !App.get('noHost')
      @get('controller').send 'redirectToFirstHost'
  ).observes 'App.noHost', 'App.hosts.loaded'

  didInsertElement: ->
    @_super()
    @handleNoHost()
    @redirectToFirstHost()
