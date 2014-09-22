App.HostImagesView = Lib.RecordListView.extend
  elementId: 'host-images-view'

  actions:
    update: ->
      @$('input.record-checkbox').attr('checked', false).change()
      @get('controller').send('update')
