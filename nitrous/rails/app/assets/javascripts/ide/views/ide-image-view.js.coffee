App.IdeImageView = Em.View.extend
  templateName: 'ide/image'
  classNames: ['ide-image-view']

  uri: (->
    if (imageData = @get 'document.content') && (contentType = @get 'document.contentType')
      "data:#{contentType};base64,#{imageData}"
    else
      @get 'document.content'
  ).property 'document.content', 'document.contentType'

  closeImage: (->
    if @get 'document.path.deleted'
      this.remove()
  ).observes 'document.path.deleted'
