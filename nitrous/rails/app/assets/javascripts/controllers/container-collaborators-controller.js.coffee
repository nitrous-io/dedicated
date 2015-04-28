App.ContainerCollaboratorsController = Em.ObjectController.extend
  recordsBinding: Em.Binding.oneWay 'model.collaborators'

  username: null

  clear: ->
    @setProperties
      pending: Em.A()
      username: null

  actions:
    add: ->
      unless @get 'username'
        return

      records = @get 'records'
      username = @get 'username'
      @set 'username', null

      if records.findBy 'username', username
        return
      url = "/containers/#{@get('model.slug')}/collaborators"
      App.Model.adapter.ajax('POST', url, {user: username}).fail (jqxhr) ->
        App.postError(jqxhr.responseJSON, 'Failed to add collaborator')

    remove: (collaborator)->
      collaborator.ajax('DELETE').fail (jqxhr) ->
        App.postError(jqxhr.responseJSON, 'Failed to remove collaborator')
