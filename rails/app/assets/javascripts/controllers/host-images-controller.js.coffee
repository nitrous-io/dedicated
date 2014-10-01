App.HostImagesController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.images'

  # for push
  image: null
  repo: null
  closePushDialog: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      imageToPush: null
      pushRepo: null
      closePushDialog: false

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

      async.series(fns)

    push: ->
      image = @get 'image'
      repo = @get 'repo'
      image.set 'pushing', true
      image.ajax('POST', 'push', repository: repo).then(=>
        App.snackbar.post("Your image “#{image.get('displayName')}” was successfully pushed.")
        @set 'closePushDialog', true
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to push!")
      ).finally =>
        image.set 'pushing', false
