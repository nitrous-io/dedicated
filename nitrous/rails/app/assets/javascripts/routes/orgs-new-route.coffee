#= require ./authed-route

App.OrgsNewRoute = App.AuthedRoute.extend
  model: ->
    controller = @controllerFor('orgsNew')
    controller.get('content') ? App.Org.create()

  activate: ->
    App.set 'title', 'New Organization'

  renderTemplate: ->
    @render 'orgs/new', outlet: 'content'
