App.HostsQueueController = Em.Controller.extend
  actions:
    cancel: ->
      Em.$.ajax(
        type: 'DELETE'
        url: "/api/v0/free_queue"
        headers: { Authorization: "Bearer #{App.get 'token'}" }
        cache: false
        contentType: false
      )
      App.set 'freeQueue', false
      App.set 'onboardingReady', true
      @transitionToRoute 'containers.new'
