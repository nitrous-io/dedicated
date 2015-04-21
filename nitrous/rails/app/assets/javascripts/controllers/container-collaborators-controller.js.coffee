App.ContainerCollaboratorsController = Em.ObjectController.extend
  recordsBinding: Em.Binding.oneWay 'model.collaborators'

  pending: null

  username: null
  saving: false

  changes: (->
    if @get('pending') && @get('pending').length > 0
      return true
    return false unless @get('records')
    x = @get('records').any (collaborator)->
      collaborator.get 'pendingRemove'
    return x
  ).property 'pending.@each', 'records.@each.pendingRemove'

  hasRecords: (->
    # TODO: @callumj fix when 'records' has content this return false
    (@get('records.length') > 0) || (@get('pending.length') > 0)
  ).property 'pending.@each', 'records.@each'

  clear: ->
    @setProperties
      pending: Em.A()
      username: null
      saving: false

  actions:
    applyChanges: ->
      @set 'saving', true
      records = @get 'records'
      pending = @get 'pending'
      ops = []

      # we don't make use of error in the callback
      # so that execution can continue regardless
      # we will then collect all the errors after the fact

      records.filterBy('pendingRemove', true).forEach (r)->
        ops.push (cb) ->
          r.ajax('DELETE').then ->
            cb(null, {record: r})
          , (jqxhr) ->
            cb(null, {error: jqxhr, record: r})

      pending.forEach (pend) =>
        pend.error = null
        data = {user: pend.username}
        ops.push (cb) =>
          App.Model.adapter.ajax('POST', "/containers/#{@get('model.slug')}/collaborators", data).then ->
            cb(null, {pending: pend})
          , (jqxhr) ->
            cb(null,  {error: jqxhr, pending: pend})

      if ops.length == 0
        @set 'saving', false
        return

      # execute all
      async.series ops, (_, results) =>
        pendingError = false
        results.forEach (result) ->
          if (err = result.error)
            if result.pending && (errDesc = err.responseJSON?.error_description)
              pendingError = true
              result.pending.error = errDesc.capitalize()
            else
              App.postError(err.responseJSON)

        if pendingError
          App.postError("There were errors adding new collaborators")

        records.reload().then ()=>
          remainders = pending.filter (pending)->
            pending.error != null
          @set 'pending', remainders
        , (jqxhr) =>
          pending.clear()
        @set 'saving', false
        return true

    add: ->
      unless @get 'username'
        return

      set = @get 'pending'
      username = @get 'username'
      @set 'username', null

      if set.findBy 'username', username
        return

      set.pushObject {username: username, error: null}

    removePending: (username)->
      set = @get 'pending'
      set.removeObject username