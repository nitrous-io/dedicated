App.HostImagesController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.images'

  actions:
    pull: ->
      @transitionToRoute 'images.pull', queryParams: { preselectHost: @get('model.slug') }

    add: ->
      @transitionToRoute 'images.new', queryParams: { preselectHost: @get('model.slug') }

    update: ->
      selected = @get 'selected'
      host = @get 'model'

      fns = selected.map (record) ->
        (cb) ->
          App.Image.pull host, record.get('repo'), (resp) ->
            cb(null)
          , (jqxhr) -> # record.ajax
            App.postError(jqxhr.responseJSON, 'Failed to update image!')
            cb(jqxhr)

      async.series fns
