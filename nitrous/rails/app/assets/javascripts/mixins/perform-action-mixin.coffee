App.PerformActionMixin = Ember.Mixin.create
  performAction: (method, action, tempState, data=null) ->
    record = @get('model')
    @performActionOnRecord(record, method, action, tempState, data)

  performActionOnRecord: (record, method, action, tempState, data=null) ->
    prevState = record.get('state')
    if tempState
      record.set 'state', tempState
    rootKey = record.constructor.rootKey
    ajax = record.ajax(method, action, data)
    ajax.then (json) ->
      if attrs = json[rootKey]
        record.load(attrs)
    , (jqxhr) -> # record.ajax
      defaultErrMsg = if action
        "Failed to #{action}! Please contact support for help if this persists."
      App.postError(jqxhr.responseJSON, defaultErrMsg)
      record.set('state', prevState)
    ajax
