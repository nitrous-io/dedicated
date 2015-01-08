#= require ./admin-route
#= require ../mixins/perform-action-mixin

App.AdminHostsRoute = App.AdminRoute.extend App.PerformActionMixin,
  activate: ->
    App.set 'regions', App.Region.fetch().value
    App.set 'plans', App.Plan.fetch().value

    App.set 'title', 'Admin - Hosts'

  model: (params) ->
    App.AdminHosts.fetch(params.provider)

  actions:
    restart: (host) ->
      @performActionOnRecord(host, 'POST', 'restart', 'restarting').then =>
        @refresh()

    terminate: (host) ->
      @performActionOnRecord(host, 'DELETE', null, 'terminating').then =>
        App.snackbar.post("Host “#{host.get('slug')}” was successfully terminated.")
        @refresh()
