App.HostIndexView = Em.View.extend
  elementId: 'host-index-view'

  hostIsRunning: Em.computed.equal 'controller.model.state', 'running'
  hostIsStopped: Em.computed.equal 'controller.model.state', 'stopped'

  createdAtUTC: (->
    @get('controller.model.createdAt')?.toUTCString()
  ).property 'controller.model.createdAt'

  actions:
    refresh: ->
      $btn = @$('.refresh-btn')
      $btn.attr 'disabled', true
      @get('controller').send 'refresh'
      window.setTimeout(->
        $btn.attr 'disabled', false
      , 1000)
