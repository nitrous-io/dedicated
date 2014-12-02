App.HackController = Em.ObjectController.extend
  queryParams: [
    'hack_repo_url'
    'hack_runtime'
    'hack_file_to_open'
  ]
  hack_repo_url: null
  hack_runtime: null
  hack_file_to_open: null

  actions:
    newBox: ->
      @transitionToRoute 'boxes.new', queryParams:
        hack_repo_url: @get('hack_repo_url')
        hack_runtime: @get('hack_runtime')
        hack_file_to_open: @get('hack_file_to_open')
    existingBox: (box) ->
      box_id = box.get('id')

      repo = @get('model')
      repo.set 'box_id', box_id
      repo.identify (json) =>
        if repo.get('state') == 'identified'
          $.ajax(
            url:  "/boxes/#{box_id}/clone_repo"
            type: 'PUT'
            data:
              repository:
                scm: repo.get('scm')
                state: repo.get('state')
                clone_url: repo.get('clone_url')
                file_to_open: @get('hack_file_to_open')
                download_branch: repo.get('download_branch')
          ).done((json) =>
            @transitionToRoute 'box.provisioning', box.get('id')
          ).fail (e) =>
            response = $.parseJSON e.responseText
            errorMessage = response.errors?.base?[0]
            @set 'errors', errorMessage
        else
          @set 'errors', 'This does not look like a valid GitHub repository'

  errors: null

  eligibleBoxes: (->
    Em.ArrayProxy.create
      content: App.get('boxes').filter (box) =>
        state = box.get('state')
        if box.get('ownBox')
          state == 'running' || state == 'stopped'
        else
          state == 'running'
  ).property 'App.boxes.@each.state'

  ineligibleBoxes: (->
    _.difference App.get('boxes.content'), @get('eligibleBoxes.content')
  ).property 'App.boxes.@each.state'
