App.HostRegistriesController = Em.Controller.extend
  actions:
    add: ->
      @transitionToRoute 'host.registries-new'
