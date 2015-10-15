App.CloneFromGitHubDialog = klass = Em.Object.extend
  repositories: null

  init: ->
    @_super()
    @set('repositories', Em.A([]))
