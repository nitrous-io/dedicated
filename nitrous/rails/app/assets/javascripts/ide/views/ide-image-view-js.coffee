App.IdeImageView = Em.View.extend
  templateName: 'ide/templates/image'
  classNames: ['ide_image_view']

  imageDataURI: (->
    if (imageData = @get 'document.content') && (contentType = @get 'document.contentType')
      "data:#{contentType};base64,#{imageData}"
  ).property 'document.content', 'document.contentType'

  closeImage: (->
    if @get 'document.path.deleted'
      this.remove()
  ).observes 'document.path.deleted'
