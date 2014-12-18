App.HomeIndexController = Em.Controller.extend
  actions:
    redirectToNewHosts: ->
      @transitionToRoute 'hosts.new'

    redirectToFirstHost: ->
      @transitionToRoute 'host.index', App.get('hosts').findBy('state', 'running')
