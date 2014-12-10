App.HomeIndexController = Em.Controller.extend
  actions:
    redirectToNewHosts: ->
      @transitionToRoute 'hosts.new'

    redirectToFirstHost: ->
      @transitionToRoute 'app.index', App.get('hosts.firstObject.slug')
