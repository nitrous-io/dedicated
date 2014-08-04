#= require ../lib/rest-adapter
#= require ../lib/simple-model/model

App.Model = klass = SM.Model.extend()

klass.reopenClass
  adapter: App.RESTAdapter.create()
