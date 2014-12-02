App.BoxesIndexBoxView = Em.View.extend
  templateName: 'templates/boxes/index-box'
  tagName: 'li'
  box: null
  classNameBindings: ['box.expanded']

  didInsertElement: ->
    @_super()
    @$().find('.explain').tooltip placement: "right"

  didClickBoxTitle: (event) ->
    @get('box').toggleProperty 'expanded'

  didClickIDE: (event) ->
    App.get('router').transitionTo 'box.ide', @get('box')

  didClickTerminal: (event) ->
    App.get('router').transitionTo 'box.terminal', @get('box')

  didClickProvisioning: (event) ->
    App.get('router').transitionTo 'box.provisioning', @get('box')

  didClickStart: (event) ->
    @get('box').start()

  didClickShutDown: (event) ->
    @get('box').stop()

  didClickRestart: (event) ->
    @get('box').restart()

  didClickTerminate: (event) ->
    @get('controller').confirmTerminate @get('box')

  didClickRetryProvisioning: (event) ->
    @get('box').provision()

  didClickRevealPublicKey: (event) ->
    @get('box').readDefaultPublicKey()

  didClickAddToGitHub: (event) ->
    @get('box').addPublicKeyToGitHub()

  didClickSettings: (event) ->
    App.get('router').transitionTo 'box.resources', @get('box')

  didClickViewSnapshots: (event) ->
    App.get('router').transitionTo 'box.snapshots.index', @get('box')

  didClickLeave: (event) ->
    @get('controller').confirmLeave @get('box')

  didClickBoxShutdownUpgrade: (event) ->
    App.util.mixpanel 'Clicked Box Shutdown Upgrade'
    App.get('router').transitionTo 'n2o.upgrade'

  didClickCPUShareUpgrade: (event) ->
    App.util.mixpanel 'Clicked CPU Share Upgrade'
    App.get('router').transitionTo 'n2o.upgrade'
