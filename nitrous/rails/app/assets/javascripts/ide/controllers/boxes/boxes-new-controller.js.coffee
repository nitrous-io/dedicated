App.BoxesNewController = Em.ObjectController.extend
  queryParams: [
    'hack_repo_url'
    'hack_runtime'
    'hack_file_to_open'
  ]
  hack_repo_url: null
  hack_runtime: null
  hack_file_to_open: null

  boxNamePlaceholder: 'my-awesome-box'
  boxNameIsDirty: false
  isIntro: false
  repo: null
  identifyTimedOut: false

  save: ->
    model = @get 'model'
    repo = @get 'repo'

    doSave = =>
      model.save().then =>
        router = App.get 'router'
        if @get 'isIntro'
          router.transitionTo 'intro.n2o'
        else
          router.transitionTo 'box.provisioning', model

    if repository_clone_url = @get('model.repository_clone_url')
      repo.set 'clone_url', repository_clone_url
      repo.identify (e) =>
        if e?
          @set 'model.saving', false
          @set 'model.errors',
            repository_clone_url: ['does not look like a valid GitHub repository']
        else
          model.set 'repository',
            clone_url: repo.get('clone_url')
            state: repo.get('state')
            scm: repo.get('scm')
            file_to_open: @get('hack_file_to_open')
            download_branch: repo.get('download_branch')
          doSave()
    else
      doSave()

  cancel: ->
    App.util.returnToSaved 'boxes.index'
