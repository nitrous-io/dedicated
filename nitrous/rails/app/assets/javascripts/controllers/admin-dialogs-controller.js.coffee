App.AdminDialogsController = Em.Controller.extend
  hostsToUpdate: null
  freigtherdVersion: null

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
