App.CloneFromGitHubDialog = klass = Em.Object.extend
  repositories: null
  loaded: false

  init: ->
    @_super()
    @set('repositories', Em.A([]))
    @set('loaded', false)
