App.BoxDomainsRoute = Em.Route.extend
  deactivate: ->
    @controller.set 'content', null
