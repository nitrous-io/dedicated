App.UploadDialog = Em.ArrayProxy.extend
  path: null
  content: null # array of App.Upload
  uploading: false
  authToken: null
  completed: (->
    if content = @get 'content'
      content.everyProperty 'completed'
  ).property 'content.@each.completed'

  init: ->
    @set 'content', Em.A()
    @_super.apply this, arguments
