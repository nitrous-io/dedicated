App.IdeBasicAuthController = Em.Controller.extend
  manageAuthDialog: null

  reset: ->
    @set 'manageAuthDialog', null

  showAuthDialog: ->
    if box = App.get 'router.boxIdeController.content'
      @set 'manageAuthDialog', App.ManageAuthDialog.create box: box

  updateAuthentication: ->
    if box = App.get 'router.boxIdeController.content'
      box.updateAuth (err) =>
        unless err
          @set 'manageAuthDialog', null
