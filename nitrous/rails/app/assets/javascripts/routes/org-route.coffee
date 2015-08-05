#= require ./authed-route

App.OrgRoute = App.AuthedRoute.extend
  model: (params) ->
    orgsController = @controllerFor 'orgs'
    if org = orgsController.get('collection').findBy('slug', params.orgSlug)
      orgsController.set 'selectedRecord', org
      org
    else
      promise = App.Org.fetchOne(slug: params.orgSlug)
      promise.then (org) ->
        orgsController.set 'selectedRecord', org
      promise

  deactivate: ->
    @controllerFor('orgs').set 'selectedRecord', null
