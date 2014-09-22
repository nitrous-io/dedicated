App.HostImagesController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.images'

  actions:
    add: ->
      @transitionToRoute 'images.new', queryParams: { preselectHost: @get('model.slug') }

    update: ->
      selected = @get 'selected'
      host = @get 'model'

      fns = selected.map (record) ->
        (cb) ->
          record.set 'updating', true
          App.Image.pull host, record.get('repo'), (resp) ->
            console.log resp
            cb(null)
          , (jqxhr) -> # record.ajax
            errMsg = jqxhr.responseJSON?.error_description
            App.snackbar.post(errMsg?.capitalize() || 'Unknown Error')
            record.set 'updating', false
            cb(jqxhr)

      async.series fns
