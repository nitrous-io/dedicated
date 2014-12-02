App.IdeCollabMessageView = Em.View.extend
  templateName: 'templates/box/ide/collab-message'
  tagName: 'li'
  classNameBindings: ['content.isChatMessage:chat-message:event-message']
  content: null

  didClickOnFileLink: ->
    if filePath = @get 'content.data'
      path = App.Path.create absolutePath: filePath
      ideSidebarController = App.get 'router.ideSidebarController'
      if path && ideSidebarController
        ideSidebarController.openPath path, source: 'collab-message'
