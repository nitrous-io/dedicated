App.HomeIndexController = Em.Controller.extend
  actions:
    redirectToNewHosts: ->
      @transitionToRoute 'hosts.new'

    redirectToFirstHost: ->
      slug = App.get('hosts').findBy('state', 'running')?.get('slug')
      if App.get('user.advanced')
        @transitionToRoute 'host.index', slug
      else
        @transitionToRoute 'app.index', slug
