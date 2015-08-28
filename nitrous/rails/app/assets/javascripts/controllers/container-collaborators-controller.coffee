App.ContainerCollaboratorsController = Em.ObjectController.extend
  recordsBinding: Em.Binding.oneWay 'model.collaborators'
  containerSlugBinding: Em.Binding.oneWay 'model.slug'

  username: null

  clear: ->
    @setProperties
      pending: Em.A()
      username: null

  actions:
    add: ->
      unless @get 'username'
        return

      containerSlug = @get 'model.slug'
      records = @get 'records'
      username = @get 'username'
      @set 'username', null

      if records.findBy 'username', username
        return
      url = "/containers/#{containerSlug}/collaborators"
      App.Model.adapter.ajax('POST', url, {user: username}).then ->
        App.snackbar.post("You added “#{username}“ to “#{containerSlug}“")
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, 'Failed to add collaborator')

    showRemoveCollaborator: (collaborator)->
      @set('collaborator', collaborator)
      ui.showModal '.remove-collaborator-modal-dialog'

    hideRemoveCollaborator: ->
      ui.hideModal '.remove-collaborator-modal-dialog'
      @set('collaborator', null)

    removeCollaborator: ()->
      ui.hideModal '.remove-collaborator-modal-dialog'
      collaborator = @get('collaborator')
      username = collaborator.get('username') || collaborator.get('pendingEmail')
      collaborator.ajax('DELETE').then ->
        App.snackbar.post("You removed “#{username}“")
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to remove “#{username}“")

    showLeaveContainer: (collaborator)->
      @set('collaborator', collaborator)
      ui.showModal '.leave-container-modal-dialog'

    hideLeaveContainer: ->
      ui.hideModal '.leave-container-modal-dialog'
      @set('collaborator', null)

    leaveContainer: ()->
      ui.hideModal '.leave-container-modal-dialog'
      containerSlug = @get 'model.slug'
      collaborator = @get('collaborator')
      collaborator.ajax('DELETE').then =>
        App.snackbar.post("You left “#{containerSlug}“")
        @transitionToRoute 'home.index'
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to leave “#{containerSlug}“")
