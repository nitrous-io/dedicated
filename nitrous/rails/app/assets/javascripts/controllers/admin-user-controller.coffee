App.AdminUserController = Em.ObjectController.extend
  actions:
    banUser: (user)->
      user.banUser().then =>
        App.snackbar.post "User #{@get('model.username')} has been banned"
        @set 'model', App.AdminUser.fetchUsers(user.get('username')).value
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to ban user.')
