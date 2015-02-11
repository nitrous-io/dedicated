App.IdeIframeView = Em.View.extend
  templateName: 'ide/iframe'
  classNames: ['ide-iframe-view']

  uri: (->
    @get 'document.content'
  ).property 'document.content', 'document.contentType'
