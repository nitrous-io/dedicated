App.PerformActionMixin = Ember.Mixin.create
  performAction: (method, action, tempState) ->
    record = @get('model')
    prevState = record.get('state')
    record.set 'state', tempState
    rootKey = record.constructor.rootKey
    ajax = record.ajax(method, action)
    ajax.then (json) ->
      if data = json[rootKey]
        record.load(data)
    , (jqxhr) -> # record.ajax
      App.postError(jqxhr.responseJSON, "Failed to #{action}!")
      record.set('state', prevState)
    ajax
