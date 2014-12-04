App.IdePathNameView = Em.View.extend
  templateName: 'ide/templates/path-name'
  tagName: 'a'
  classNameBindings: ['path.loading:loading', 'path.beingDeleted:deleted']

  path: null
