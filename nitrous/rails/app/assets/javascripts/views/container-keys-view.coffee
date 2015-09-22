App.ContainerKeysView = Lib.RecordListView.extend
  elementId: 'container-keys-view'

  showAddGitHubKey: ( ->
    !@get('controller.isKeyAddedToGitHub')
  ).property 'controller.isKeyAddedToGitHub'

  addToGithubDisabled: ( ->
    @get('controller.saving') || @get('controller.removing') || @get('controller.isKeyAddedToGitHub')
  ).property 'controller.saving', 'controller.removing', 'controller.isKeyAddedToGitHub'

  removeKeyDisabled: ( ->
    @get('controller.saving') || @get('controller.removing') ||
      !@get('controller.isKeyAddedToGitHub') || !App.get('userProfile.canDeleteGitHubPublicKeys')
  ).property 'controller.saving', 'controller.removing', 'controller.isKeyAddedToGitHub'

  $removeContainerKeyModal: ->
    $('#confirm-remove-container-key-modal-dialog')

  actions:
    askRemoveContainerKey: ->
      ui.showModal(@$removeContainerKeyModal())

    cancelRemoveContainerKey: ->
      ui.hideModal(@$removeContainerKeyModal())

    confirmRemoveContainerKey: ->
      ui.hideModal(@$removeContainerKeyModal())
      @get('controller').send 'removeFromGitHub'

