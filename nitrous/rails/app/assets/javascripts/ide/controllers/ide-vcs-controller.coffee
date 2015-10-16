App.IdeVCSController = Em.Controller.extend
  needs: ['ideConsole']

  cloneFromGitHubDialog: null
  selectedRepository: null

  reset: ->
    if @get 'cloneFromGitHubDialog'
      @set 'cloneFromGitHubDialog.hide', true
      @set 'cloneFromGitHubDialog', null

  showCloneFromGitHubDialog: ->
    cloneFromGitHubDialog = App.CloneFromGitHubDialog.create loaded: false
    @set 'cloneFromGitHubDialog', cloneFromGitHubDialog
    Em.$.ajax(
      type: 'GET'
      url: '/api/v0/github/repos'
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    ).done (data) =>
      cloneFromGitHubDialog.set('loaded', true)
      repos = cloneFromGitHubDialog.get('repositories')
      for repo in data.repos
        repos.pushObject(repo)

  openTerminalAndCloneRepo: (gitUrl) ->
    # git://github.com/username/project-name.git => project-name
    matches = gitUrl.match(/\/([^\/]+)\.git/)
    if matches
      cloneDir = "/home/nitrous/code/#{matches[1]}"
      tab = App.ConsoleSession.create(initCommand: "git clone #{gitUrl} #{cloneDir} && cd #{cloneDir}")
      @get('controllers.ideConsole').addTab tab, {}
