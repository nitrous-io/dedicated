App.HomeIndexController = Em.Controller.extend
  actions:
    redirectToNewHosts: ->
      @transitionToRoute 'hosts.new'

    redirectToFirstHost: ->
      slug = App.get('hosts').findBy('state', 'running')?.get('slug')
      if !slug 
        slug = App.get('hosts.firstObject.slug')
      
      @transitionToRoute 'host.index', slug
