App.BetaController = Em.Controller.extend
  login: null
  switching: false
  closeSwitchUser: false

  clear: ->
    @setProperties
      login: null
      switching: false
      closeSwitchUser: false

  actions:
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
