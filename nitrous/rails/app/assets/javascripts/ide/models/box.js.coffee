#= require ./base-model

App.Box = klass = App.BaseModel.extend
  name: Em.attr()
  state: Em.attr()

  rootKey: 'box'
  collectionKey: 'boxes'
  url: '/boxes'
