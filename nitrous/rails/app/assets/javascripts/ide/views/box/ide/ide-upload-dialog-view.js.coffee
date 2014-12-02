App.IdeUploadDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/box/ide/upload-dialog'
  elementId: 'ide_upload_dialog_view'
  contentBinding: 'controller.uploadDialog' # controler = ideSidebarController
  classNameBindings: ['content.uploading:uploading']

  dragAndDropCapable: (->
    Modernizr.draganddrop
  ).property()

  uploadURI: (->
    if boxServerURI = App.get 'router.boxIdeController.content.box_server_uri'
      "#{boxServerURI}/upload"
  ).property 'App.router.boxIdeController.content.box_server_uri'

  didInsertElement: ->
    @_super()
    $files = @$ '.files'
    @$('.file_input').fileupload
      url: @get('uploadURI')
      formData: =>
        [{ name: "auth_token", value: @get('content.authToken') }]
      dataType: 'json'
      maxFileSize: 20 * 1000 * 1000
      singleFileUploads: true
      sequentialUploads: true
      dropZone: @$('.drop_zone')
      add: _.bind(@didAddUpload, this)
      progress: _.bind(@progressDidChange, this)
      done: _.bind(@uploadDidSucceed, this)
      fail: _.bind(@uploadDidFail, this)

  didAddUpload: (event, data) ->
    content = @get 'content'
    fileName = data.files[0].name
    if content && !content.get('uploading') && !content.findProperty('fileName', fileName)
      content.addObject App.Upload.create(data: data)

  progressDidChange: (event, data) ->
    if content = @get 'content'
      fileName = data.files[0].name
      upload = content.findProperty 'fileName', fileName
      upload.set 'progressInterval', data.progressInterval

  uploadDidSucceed: (event, data) ->
    if content = @get 'content'
      fileName = data.files[0].name
      upload = content.findProperty 'fileName', fileName
      upload.set 'completed', true
      token = data.result.tokens[0]
      path = content.get 'path.absolutePath'
      filePath = if path.length > 0
        "#{path}/#{fileName}"
      else
        fileName
      @get('controller').sendUploadToken filePath, token

  uploadDidFail: (event, data) ->
    if content = @get 'content'
      fileName = data.files[0].name
      upload = content.findProperty 'fileName', fileName
      upload.set 'completed', true
      upload.set 'failed', true
      @get('controller').didFinishAnUpload()

  didClickRemoveUpload: (upload) ->
    if content = @get 'content'
      content.removeObject upload

  didClickUpload: ->
    @get('controller').beginUpload()

App.IdeUploadDialogDropZoneView = Em.View.extend
  classNames: ['drop_zone']
  classNameBindings: ['draggingOver:dragging_over']
  draggingOver: false

  dragOver: (event) ->
    @set 'draggingOver', true

  dragLeave: (event) ->
    @set 'draggingOver', false

  drop: (event) ->
    @set 'draggingOver', false

