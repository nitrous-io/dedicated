App.ContainerKeysView = Lib.RecordListView.extend
  elementId: 'container-keys-view'

  addToGithubDisabled: (->
    @get('controller.saving') || @get('controller.githubKey')
  ).property 'controller.saving', 'controller.githubKey'

  removeKeyDisabled: (->
    @get('controller.saving') || @get('controller.removing') || !@get('controller.githubKey')
  ).property 'controller.saving', 'controller.removing', 'controller.githubKey'

  $removeContainerKeyModal: ->
    $('#confirm-remove-container-key-modal-dialog')

  actions:
    askRemoveContainerKey: ->
      ui.showModal(@$removeContainerKeyModal())

    cancelRemoveContainerKey: ->
      ui.hideModal(@$removeContainerKeyModal())

    confirmRemoveContainerKey: ->
      ui.hideModal(@$removeContainerKeyModal())
      # @get('controller').send 'removeFromGitHub'

