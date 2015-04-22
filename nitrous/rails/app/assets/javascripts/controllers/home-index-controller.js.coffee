App.HomeIndexController = Em.Controller.extend
  actions:
    handleFreeTier: ->
      App.FreeQueue.load().then =>
        return unless App.get("freeQueue")
        @transitionToRoute "free-tier", queryParams: { skip: true }

    handleNoHost: ->
      unless App.get "freeQueue"
        @transitionToRoute 'containers.new'

      if App.get('user.firstTime')
        App.set 'onboardingReady', true

    redirectToFirstHost: ->
      slug = App.get('hosts').findBy('state', 'running')?.get('slug')
      if !slug
        slug = App.get('hosts.firstObject.slug')

      @transitionToRoute 'host.index', slug

    redirectToHostNew: ->
      @transitionToRoute 'hosts.new'

    redirectToContainerNew: ->
      @transitionToRoute 'containers.new'
