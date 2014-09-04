Lib.RecordListView = Em.View.extend
  updatedSelectedIds: (->
    Em.run.scheduleOnce 'afterRender', =>
      selected = @$('input.record-checkbox:checked').map (i, el) ->
        parseInt(el.value, 10)
      @set 'controller.selectedIds', $.makeArray(selected)
  ).observes 'controller.records.@each'

  didInsertElement: ->
    @_super()
    @$().on 'change.records', 'input.record-checkbox', =>
      @updatedSelectedIds()

  willDestroyElement: ->
    @_super()
    @$().off 'change.records'
