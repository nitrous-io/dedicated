#= require ./admin-route
#= require ../mixins/perform-action-mixin

App.AdminHostsRoute = App.AdminRoute.extend App.PerformActionMixin,
  provider: null
  activate: ->
    App.set 'title', 'Admin - Hosts'

  model: (params) ->
    @set('provider', params.provider)
    App.AdminHosts.fetch(params.provider)

  actions:
    restart: (host) ->
      App.snackbar.post("Restarting host “#{host.get('slug')}”.")
      @performActionOnRecord(host, 'POST', 'restart', 'restarting').then =>
        App.snackbar.post("Host “#{host.get('slug')}” was successfully restarted.")
        @refresh()

    terminate: (host) ->
      @performActionOnRecord(host, 'DELETE', null, 'terminating').then =>
        App.snackbar.post("Host “#{host.get('slug')}” was successfully terminated.")
        @refresh()

    terminateUnaccounted: (host) ->
      Em.$.ajax(
        type: 'DELETE'
        url: "/api/v0/admin/terminate_unaccounted_host?provider=#{@get('provider')}&provider_identifier=#{host.get('id')}"
        cache: false
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).then =>
        App.snackbar.post('Unaccounted host was successfully terminated.')
        @refresh()
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to remove unaccounted host!')
