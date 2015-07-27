Lib.RecordListView = Em.View.extend
  classNames: ['record-list-view']

  multipleSelected: (->
    @get('controller.selected.length') > 1
  ).property 'controller.selected.length'

  updatedSelectedIds: (->
    Em.run.scheduleOnce 'afterRender', =>
      jq = @$('input.record-checkbox:checked:not(:disabled)')
      unless jq
        return
      selected = jq.map (i, el) ->
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

  actions:
    askRemove: ->
      ui.showModal(@$('#confirm-remove-modal-dialog'))

    cancelRemove: ->
      ui.hideModal(@$('#confirm-remove-modal-dialog'))

    confirmRemove: ->
      ui.hideModal(@$('#confirm-remove-modal-dialog'))
      @get('controller').send 'remove'
