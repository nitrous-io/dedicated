App.IdeImageURLView = Em.View.extend
  templateName: 'ide/image-url'
  classNames: ['ide-image-view']

  urlBinding: 'document.content'

  closeImage: (->
    if @get 'document.path.deleted'
      this.remove()
  ).observes 'document.path.deleted'
