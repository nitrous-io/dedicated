App.AdminDialogsController = Em.Controller.extend
  login: null
  switching: false
  closeSwitchUser: false

  init: ->
    @_super.apply this, arguments

    App.on 'switch-to-user', (username)=>
      @set 'login', username
      @send 'switchUser'

  clear: ->
    @setProperties
      login: null
      switching: false
      closeSwitchUser: false

  actions:
    viewUser: ->
      @transitionToRoute 'admin.user', @get('login')
      App.trigger 'hide-view-user-dialog'

    switchUser: ->
      @set 'switching', true

      data =
        login: @get 'login'

      App.User.adapter.ajax('POST', '/admin/switch_user', data).done((d) =>
        # clear mixpanel cookie to avoid tracking
        window.mixpanel?.cookie?.clear?()

        # invalidate App.user since we are different user now
        App.set('user', null)

        unless App.get('adminToken')
          # If App.adminToken is already set, it means we have switched from
          # one admin to another admin user, and try to switch once more as
          # the switched admin user. Hence we don't override the original
          # adminToken so that when switching back it is the first original admin.
          App.setAdminToken(App.get('token'))

        App.setToken(d.access_token)

        @set 'closeSwitchUser', true
        App.snackbar.post("Successfully logged in as #{@get('login')}")

        Em.run.next ->
          location.href = "/"
      ).fail (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to switch user!")
        @set 'switching', false
