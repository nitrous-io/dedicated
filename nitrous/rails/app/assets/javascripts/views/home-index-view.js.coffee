App.HomeIndexView = Em.View.extend
  elementId: 'home-index-view'

  redirectToNewHosts: (->
    if App.get 'noHost'
      @get('controller').send 'redirectToNewHosts'
  ).observes 'App.noHost'

  redirectToFirstHost: (->
    if App.get('hosts.loaded') && !App.get('noHost')
      @get('controller').send 'redirectToFirstHost'
  ).observes 'App.noHost', 'App.hosts.loaded'

  didInsertElement: ->
    @_super()
    @redirectToNewHosts()
    @redirectToFirstHost()
