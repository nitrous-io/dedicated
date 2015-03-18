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
