App.HomeIndexController = Em.Controller.extend
  actions:
    handleNoHost: ->
      if App.get('user.firstTime')
        App.set 'onboardingReady', true

      if shared = App.get('sharedContainers.firstObject.slug')
        @transitionToRoute 'container.index', shared
        return

      unless App.get "freeQueue"
        @transitionToRoute 'containers.new'

    redirectToFirstHost: ->
      slug = App.get('hosts').findBy('state', 'running')?.get('slug')
      if !slug
        slug = App.get('hosts.firstObject.slug')

      @transitionToRoute 'host.index', slug

    redirectToHostNew: ->
      @transitionToRoute 'hosts.new'

    redirectToContainerNew: ->
      @transitionToRoute 'containers.new'
