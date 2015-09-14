App.ContainerKeysController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.userKeys'
  containerKeysBinding: Em.Binding.oneWay 'model.containerKeys'

  saving: false

  error: false
  notRunningError: false
  githubKey: null

  clear: ->
    @_super.apply this, arguments
    @getExistingGithubKeys()
    @setProperties
      error: false
      notRunningError: false
      saving: false

  getExistingGithubKeys: ->
    @get('model').ajax('GET', 'container_keys/github').then( (data)=>
      @set('githubKey', data.keys[0])
    , (jqxhr) ->
    ).finally =>
      @set 'saving', false

  actions:
    add: ->
      @transitionToRoute 'container.keys-new'

    addToGitHub: ->
      @set 'saving', true
      @get('model').ajax('POST', 'container_keys/github').then(->
        App.snackbar.post("Key was successfully added to GitHub.")
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to add container public key to GitHub!")
      ).finally =>
        @set 'saving', false

    upgradePaid: ->
      subscriptionSlug = @get('host.subscriptionSlug')
      @transitionToRoute 'subscriptions.upgrade', subscriptionSlug
