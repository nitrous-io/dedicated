Ember.notifyPropertyChange = (obj, keyName) ->
  Ember.propertyWillChange(obj, keyName)
  Ember.propertyDidChange(obj, keyName)
