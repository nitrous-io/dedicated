#= require ./authed-route

App.OrgRoute = App.AuthedRoute.extend
  controllerName: 'org'

  model: (params) ->
    @controllerFor('orgs').get('collection').findBy('slug', params.orgSlug)?.reload() ?
      App.Org.fetchOne(slug: params.orgSlug)

  renderTemplate: ->
    @render 'org', outlet: 'content'

  deactivate: ->
    @controller.set 'model', null
