Lib.RecordListController = Em.ObjectController.extend
  removing: false
  selectedIds: null
  records: null

  selected: (->
    ids = @get 'selectedIds'
    records = @get 'records'
    if ids && records
      ids.map((id) ->
        records.findBy 'id', id
      ).compact()
  ).property 'selectedIds'

  noneSelected: Em.computed.empty 'selectedIds'

  clear: ->
    @setProperties
      removing: false
      selectedIds: null

  actions:
    remove: ->
      @set 'removing', true
      selected = @get 'selected'
      records = @get 'records'

      fns = selected.map (record) ->
        (cb) ->
          record.ajax('DELETE').then ->
            cb(null)
          , (jqxhr) -> # record.ajax
            App.postError(jqxhr.responseJSON)
            cb(jqxhr)

      async.series fns, (err) =>
        records.reload()
        @set 'removing', false
