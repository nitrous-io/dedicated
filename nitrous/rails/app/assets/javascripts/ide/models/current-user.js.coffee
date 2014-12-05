#= require ./base-model

App.CurrentUser = klass = App.BaseModel.extend
  username: Em.attr()
  email: Em.attr()
  full_name: Em.attr()
  created_at: Em.attr()
  admin: Em.attr()
  confirmed_at: Em.attr()
  user_profile: Em.attr()

  init: ->
    @_super.apply this, arguments

klass.reopenClass
  adapter: null # do REST manually for this class
  rootKey: 'user'
