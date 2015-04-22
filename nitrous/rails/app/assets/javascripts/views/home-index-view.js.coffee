App.HomeIndexView = Em.View.extend
  elementId: 'home-index-view'

  handleFreeTier: (->
    return unless App.get('noHost')
    if App.get("billingProfile.free")
      @get('controller').send 'handleFreeTier'
  ).observes 'App.billingProfile.free', 'App.noHost'

  handleNoHost: (->
    if App.get 'noHost'
      @get('controller').send 'handleNoHost'
  ).observes 'App.noHost'

  redirectToFirstHost: (->
    if App.get('hosts.loaded') && !App.get('noHost')
      @get('controller').send 'redirectToFirstHost'
  ).observes 'App.noHost', 'App.hosts.loaded'

  redirectToHostNew: ->
    @get('controller').send 'redirectToHostNew'

  redirectToContainerNew: ->
    @get('controller').send 'redirectToContainerNew'

  didInsertElement: ->
    @_super()
    if App.get("billingProfile.free")
      if App.get('hostSaving')
        @redirectToHostNew()
        return

      if App.get('containerSaving')
        @redirectToContainerNew()
        return

    @handleFreeTier()
    @handleNoHost()
    @redirectToFirstHost()
