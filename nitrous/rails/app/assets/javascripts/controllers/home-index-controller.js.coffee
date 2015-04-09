App.HomeIndexController = Em.Controller.extend
  actions:
    handleNoHost: ->
      @transitionToRoute 'containers.new'
      App.set 'onboardingReady', true

    redirectToFirstHost: ->
      slug = App.get('hosts').findBy('state', 'running')?.get('slug')
      if !slug
        slug = App.get('hosts.firstObject.slug')

      @transitionToRoute 'host.index', slug
