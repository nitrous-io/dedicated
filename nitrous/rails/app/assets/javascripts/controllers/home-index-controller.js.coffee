App.HomeIndexController = Em.Controller.extend
  actions:
    redirectToNewHosts: ->
      @transitionToRoute 'hosts.new'

    redirectToFirstHost: ->
      slug = App.get('hosts').findBy('state', 'running')?.get('slug')
      @transitionToRoute 'host.index', slug
