App.HostsSelectOwnerComponent = Em.Component.extend
  layoutName: 'components/hosts/select-owner'
  classNames: ['hosts-select-owner', 'ui', 'select']

  name: null
  disabled: null
  value: null

  user: null
  orgs: null

  _initialValue: null

  didInsertElement: ->
    @_super.apply this, arguments
    @set '_initialValue', @get('value')
    @valueDidChange()

  valueDidChange: (->
    val = @get('value') || '(user)'
    return if @$('select').val() == val
    @$("option[value!='#{val}']").attr(selected: false)
    @$("option[value='#{val}']").attr(selected: true)
  ).observes 'value'

  change: (e) ->
    val = $(e.target).val()
    val = null if val == '(user)'
    @set 'value', val
