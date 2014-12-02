App.BoxesIndexController = Em.ArrayController.extend
  confirmTerminateDialog: null
  confirmLeaveDialog: null

  hasForceShutdownBoxes: (->
    return false if App.get('subscription.active')
    content = @get('content')
    content.some (box) -> box.get('force_shutdown')
  ).property 'content.@each.force_shutdown', 'App.subscription.active'

  refresh: ->
    @get('content').reload()

  confirmTerminate: (box) ->
    @set 'confirmTerminateDialog', App.ConfirmTerminateDialog.create box: box

  confirmLeave: (box) ->
    @set 'confirmLeaveDialog', App.ConfirmLeaveDialog.create box: box

  leave: (box) ->
    box.removeCollaborator App.get('currentUser.username'), =>
      @refresh()
