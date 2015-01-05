#= require ./admin-route

App.AdminHostsRoute = App.AdminRoute.extend
  activate: ->
    App.set 'regions', App.Region.fetch().value
    App.set 'plans', App.Plan.fetch().value

    App.set 'title', 'Admin - Hosts'

  model: (params) ->
    App.AdminHosts.fetch(params.provider)
