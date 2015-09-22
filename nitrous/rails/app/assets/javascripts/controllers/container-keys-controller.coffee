App.ContainerKeysController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.userKeys'
  containerKeysBinding: Em.Binding.oneWay 'model.containerKeys'

  saving: false

  error: false
  notRunningError: false
  isKeyAddedToGitHub: false

  clear: ->
    @_super.apply this, arguments
    @checkIfKeyIsAddedToGitHub()
    @setProperties
      error: false
      notRunningError: false
      saving: false
      isKeyAddedToGitHub: false

  checkIfKeyIsAddedToGitHub: ->
    @get('model').ajax('GET', 'container_keys/github').then( (data) =>
      return unless data.keys
      for githubKey in data.keys
        containerKey = @get('containerKeys').find( (item) =>
          if item.get('key').indexOf(githubKey) > -1
            return true
        )

        if containerKey
          @set("isKeyAddedToGitHub", true)
          break

    , (jqxhr) ->
    ).finally =>
      @set 'saving', false

  actions:
    add: ->
      @transitionToRoute 'container.keys-new'

    addToGitHub: ->
      @set 'saving', true
      @get('model').ajax('POST', 'container_keys/github').then(=>
        @set 'isKeyAddedToGitHub', true
        App.snackbar.post("Key was successfully added to GitHub.")
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to add container public key to GitHub!")
      ).finally =>
        @set 'saving', false

    upgradePaid: ->
      subscriptionSlug = @get('host.subscriptionSlug')
      @transitionToRoute 'subscriptions.upgrade', subscriptionSlug

    removeFromGitHub: ->
      @set 'removing', true
      @get('model').ajax('DELETE', 'container_keys/github').then(=>
        @set 'isKeyAddedToGitHub', false
        App.snackbar.post("Key successfully removed from GitHub.")
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to remove container public key from GitHub!")
      ).finally =>
        @set 'removing', false

