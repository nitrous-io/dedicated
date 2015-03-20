App.AdminAmazonHostsController = Em.Controller.extend
  clear: ->
    @fetchHosts()

  fetchHosts: ->
    Em.$.ajax(
      type: 'GET'
      url: '/api/v0/admin/amazon/hosts'
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    )
    .then (data) =>
      @set 'hosts', data.hosts

  actions:
    terminateUnaccounted: (host) ->
      console.log('terminating unaccounted host', host)
      hostId = host.id
      hostRegion = host.region
      ip = host.ip_address

      Em.$.ajax(
        type: 'DELETE'
        url: "/api/v0/admin/terminate_unaccounted_host?provider=amazon&provider_identifier=#{hostId}&region=#{hostRegion}&ip_address=#{ip}"
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).then =>
        App.snackbar.post('Unaccounted host was successfully terminated.')