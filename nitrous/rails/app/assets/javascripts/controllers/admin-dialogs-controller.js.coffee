App.AdminDialogsController = Em.Controller.extend
  hostsToUpdate: null
  freigtherdVersion: null
  login: null
  switching: false
  closeSwitchUser: false

  clear: ->
    @setProperties
      login: null
      switching: false
      closeSwitchUser: false


  actions:
    updateHosts: ->
      hostsToUpdate = @get('hostsToUpdate')
      freighterdVersion = @get('freighterdVersion')

      if hostsToUpdate && freighterdVersion
        reqs = hostsToUpdate.map (host) ->
          Em.$.ajax
            type: 'PUT'
            url: "/api/v0/admin/host/#{host.get('slug')}"
            cache: false
            data: { freighterd_version: freighterdVersion }
            headers: { Authorization: "Bearer #{App.get 'token'}" }
        Promise.all(reqs).then ->
          App.trigger 'hide-update-hosts'
          App.snackbar.post('Updated given hosts.')
        , (jqxhr) =>
          App.postError(jqxhr.responseJSON, 'Failed to update some hosts. Please try again.')

    switchUser: ->
      @set 'switching', true

      data =
        login: @get 'login'

      App.User.adapter.ajax('POST', '/admin/switch_user', data).done((d) =>
        App.set 'token', d.access_token
        @set 'closeSwitchUser', true
        App.snackbar.post("Successfully logged in as #{@get('login')}")

        Em.run.next ->
          location.href = "/"
      ).fail (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to switch user!")
        @set 'switching', false
