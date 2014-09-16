App.HostImagesController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.images'

  actions:
    add: ->
      @transitionToRoute 'images.new', queryParams: { preselectHost: @get('model.slug') }
