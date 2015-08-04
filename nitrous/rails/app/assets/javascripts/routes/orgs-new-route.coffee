#= require ./authed-route

App.OrgsNewRoute = App.AuthedRoute.extend
  controllerName: 'orgs'

  activate: ->
    App.set 'title', 'New Organization'
    @subOrgsNewCreated = PubSub.subscribe 'orgs-new.created', (msg, model) =>
      console.log('created! yay!')

  deactivate: ->
    PubSub.unsubscribe @subOrgsNewCreated

  renderTemplate: ->
    @render 'orgs/new', outlet: 'content'

  setupController: (controller) ->
    controller.set('newRecord', controller.get('newRecord') ? App.Org.create())
