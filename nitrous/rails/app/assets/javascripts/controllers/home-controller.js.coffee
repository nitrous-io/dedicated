App.HomeController = Em.Controller.extend
  actions:
    redirectToFirstContainer: ->
      if c = App.get('containers.firstObject')
        @transitionToRoute 'container.index', c.get 'slug'