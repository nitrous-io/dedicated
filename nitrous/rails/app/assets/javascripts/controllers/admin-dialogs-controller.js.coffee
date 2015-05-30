App.AdminDialogsController = Em.Controller.extend
  hostsToUpdate: null
  freigtherdVersion: null
  login: null
  switching: false
  closeSwitchUser: false

  crediting: false
  creditUser: null
  creditTypes: null
  creditAmount: null
  selectedCreditType: null

  creditTypesDidChange: (->
    return unless (f = @get('creditTypes.firstObject'))
    if @get('selectedCreditType') == null
      @set 'selectedCreditType', f.get("name")
  ).observes 'creditTypes.@each', 'selectedCreditType'

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
      crediting: false
      creditUser: null
      creditAmount: null
      selectedCreditType: null

  actions:
    loadCreditTypes: ->
      @set 'creditTypes', App.AdminCreditType.fetch().value

    applyCredits: (ev)->
      type = @get('creditTypes').findBy('name', @get('selectedCreditType'))
      unless type
        App.postError("Credit Type not selected")
        return


      amnt = parseInt(@get('creditAmount'))
      if amnt == 0
        App.postError("Amount cannot be zero")
        return

      if amnt > type.get('defaultValue')
        App.postError("Amount must be less than Default Value")
        return

      @set 'crediting', true

      data =
        username: @get 'creditUser'
        credit_type: @get 'selectedCreditType'
        amount: amnt

      App.AdminCreditType.adapter.ajax('POST', '/admin/credit', data).then (resp)=>
        App.snackbar.post("#{data.username} credited, total credits: #{resp.available_credits}")

        @setProperties
          crediting: false
          creditUser: null
          creditAmount: null
        Em.$('#credit-username').focus()
      , (jqxhr)=>
        @set 'crediting', false
        App.postError(jqxhr.responseJSON, 'Failed to credit user. Please try again.')


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

    viewUser: ->
      @transitionToRoute 'admin.user', @get('login')
      App.trigger 'hide-view-user-dialog'

    switchUser: ->
      @set 'switching', true

      data =
        login: @get 'login'

      App.User.adapter.ajax('POST', '/admin/switch_user', data).done((d) =>
        unless App.get('adminToken')
          # If App.adminToken is already set, it means we have switched from
          # one admin to another admin user, and try to switch once more as
          # the switched admin user. Hence we don't override the original
          # adminToken so that when switching back it is the first original admin.
          App.set('adminToken', App.get('token'))

        App.set('token', d.access_token)

        @set 'closeSwitchUser', true
        App.snackbar.post("Successfully logged in as #{@get('login')}")

        Em.run.next ->
          location.href = "/"
      ).fail (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to switch user!")
        @set 'switching', false
